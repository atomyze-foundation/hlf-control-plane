package delivery

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/peer"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	cutil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protoutil"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type peerCli struct {
	id    protoutil.Signer
	l     *zap.Logger
	peers []*peer.Peer
	pool  peer.Pool
}

func (c *peerCli) SubscribeTx(ctx context.Context, channelName string, txID string) (pb.TxValidationCode, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	resChan := make(chan pb.TxValidationCode)

	for _, p := range c.peers {
		p := p
		dCli, err := c.pool.GetDeliver(ctx, p)
		if err != nil && errors.Is(err, context.Canceled) {
			c.l.Error("peer deliver failed", zap.Error(err), zap.String("peer", p.String()))
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.waitFirst(ctx, cancel, p, dCli, channelName, txID, resChan)
		}()
	}

	var code pb.TxValidationCode
	go func() {
		for res := range resChan {
			code = res
		}
	}()

	wg.Wait()
	close(resChan)

	return code, nil
}

func (c *peerCli) SubscribeTxAll(ctx context.Context, channelName string, txID string) error {
	var wg sync.WaitGroup
	resChan := make(chan pb.TxValidationCode)

	for _, p := range c.peers {
		c.l.Debug("add peer to tx wait", zap.String("peer", p.String()))
		wg.Add(1)
		dCli, err := c.pool.GetDeliver(ctx, p)
		if err != nil && errors.Is(err, context.Canceled) {
			c.l.Error("peer deliver failed", zap.Error(err), zap.String("peer", p.String()))
			return fmt.Errorf("get peer deliver: %w", err)
		}
		go c.waitTx(ctx, &wg, p, dCli, channelName, txID, resChan)
	}

	txCodes := make([]pb.TxValidationCode, 0, len(c.peers))
	go func() {
		wg.Wait()
		close(resChan)
	}()

	for res := range resChan {
		txCodes = append(txCodes, res)
	}

	if len(txCodes) != len(c.peers) {
		return fmt.Errorf("expected %d responses, got: %d", len(c.peers), len(txCodes))
	}

	for _, code := range txCodes {
		if code != pb.TxValidationCode_VALID {
			return fmt.Errorf("tx invalid with code: %s", code.String())
		}
	}

	return nil
}

func (c *peerCli) waitFirst(ctx context.Context, stopFunc context.CancelFunc, p *peer.Peer, cli pb.DeliverClient, channelName string, txID string, resChan chan<- pb.TxValidationCode) {
	defer stopFunc()
	code, err := c.getTxValidationCode(ctx, cli, channelName, txID, p)
	if err != nil {
		if s, ok := status.FromError(errors.Unwrap(err)); ok {
			if s.Code() != codes.Canceled {
				c.l.Error("get tx validation status error", zap.String("status", s.Code().String()))
			}
			return
		}
		c.l.Error("get tx validation code error", zap.Error(err))
	}
	resChan <- code
}

func (c *peerCli) waitTx(ctx context.Context, wg *sync.WaitGroup, p *peer.Peer, cli pb.DeliverClient, channelName string, txID string, resChan chan<- pb.TxValidationCode) {
	defer wg.Done()
	code, err := c.getTxValidationCode(ctx, cli, channelName, txID, p)
	if err != nil && !errors.Is(err, context.Canceled) {
		c.l.Error("get tx validation code error", zap.Error(err))
	}
	resChan <- code
}

func (c *peerCli) getTxValidationCode(ctx context.Context, cli pb.DeliverClient, channelName string, txID string, p *peer.Peer) (pb.TxValidationCode, error) {
	dCli, err := cli.DeliverFiltered(ctx)
	if err != nil {
		return -1, fmt.Errorf("get deliver filtered: %w", err)
	}

	c.l.Debug("deliver stream opened")
	defer func() {
		if err = dCli.CloseSend(); err != nil {
			c.l.Error("close deliver filtered stream error", zap.String("peer", p.String()), zap.Error(err))
			return
		}
		c.l.Debug("deliver stream closed")
	}()

	c.l.Debug("send envelope for start delivering blocks")
	if err = c.sendEnvelope(dCli, channelName, p); err != nil {
		return -1, fmt.Errorf("send envelope error: %w", err)
	}

	if ready := util.FromContext(ctx); ready != nil {
		select {
		case <-ready:
			break
		default:
			close(ready)
		}
	}

	for {
		resp, err := dCli.Recv()
		if err != nil {
			c.l.Error("deliver stream error", zap.Error(err))
			return -1, fmt.Errorf("receive error: %w", err)
		}

		switch r := resp.Type.(type) {
		case *pb.DeliverResponse_FilteredBlock:
			for _, tx := range r.FilteredBlock.FilteredTransactions {
				c.l.Debug("got transaction", zap.String("txId", tx.Txid), zap.String("waitTxId", txID), zap.String("code", tx.TxValidationCode.String()))
				if tx.Txid == txID {
					return tx.TxValidationCode, nil
				}
			}
		case *pb.DeliverResponse_Status:
			return -1, fmt.Errorf("deliver stopped before tx results with status: %s", r.Status.String())
		default:
			c.l.Error("unknown response", zap.Reflect("response", r))
		}
	}
}

func (c *peerCli) sendEnvelope(cli pb.Deliver_DeliverFilteredClient, channelName string, p *peer.Peer) error {
	var tlsCertHash []byte
	if p.ClientCertificate.Certificate[0] != nil {
		tlsCertHash = cutil.ComputeSHA256(p.ClientCertificate.Certificate[0])
	}
	env, err := util.GetSeekNewestEnvelope(channelName, c.id, tlsCertHash)
	if err != nil {
		return fmt.Errorf("get seek envelope: %w", err)
	}

	return cli.Send(env)
}

func NewPeer(logger *zap.Logger, pool peer.Pool, peers []*peer.Peer, id protoutil.Signer) Client {
	return &peerCli{l: logger.Named("delivery"), pool: pool, peers: peers, id: id}
}
