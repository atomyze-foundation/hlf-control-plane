package util

import (
	"context"
	"fmt"
	"math"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// GetSeekEnvelope creates a signed envelope containing a seek request for a specific channel.
func GetSeekEnvelope(
	channelName string,
	id protoutil.Signer,
	seekInfo *orderer.SeekInfo,
	tlsCertHash []byte,
) (*common.Envelope, error) {
	env, err := protoutil.CreateSignedEnvelopeWithTLSBinding(
		common.HeaderType_DELIVER_SEEK_INFO,
		channelName,
		id,
		seekInfo,
		int32(0),
		uint64(0),
		tlsCertHash,
	)
	if err != nil {
		return nil, fmt.Errorf("create envelope: %w", err)
	}

	return env, nil
}

// GetSeekOldestEnvelope creates a seek request envelope for the oldest available block on a channel.
func GetSeekOldestEnvelope(channelName string, id protoutil.Signer) (*common.Envelope, error) {
	pos := &orderer.SeekPosition{
		Type: &orderer.SeekPosition_Oldest{
			Oldest: &orderer.SeekOldest{},
		},
	}
	return GetSeekEnvelope(channelName, id, &orderer.SeekInfo{
		Start:    pos,
		Stop:     pos,
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	}, nil)
}

// GetSeekNewestEnvelope creates a seek request envelope for the newest available block on a channel.
func GetSeekNewestEnvelope(channelName string, id protoutil.Signer, tlsCertHash []byte) (*common.Envelope, error) {
	start := &orderer.SeekPosition{
		Type: &orderer.SeekPosition_Newest{
			Newest: &orderer.SeekNewest{},
		},
	}

	stop := &orderer.SeekPosition{
		Type: &orderer.SeekPosition_Specified{
			Specified: &orderer.SeekSpecified{
				Number: math.MaxUint64,
			},
		},
	}
	return GetSeekEnvelope(channelName, id, &orderer.SeekInfo{
		Start:    start,
		Stop:     stop,
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	}, tlsCertHash)
}

// GetBlockFromDeliverClient receives a common.Block from an orderer.AtomicBroadcast_DeliverClient
// and returns it.
func GetBlockFromDeliverClient(deliverCli orderer.AtomicBroadcast_DeliverClient) (*common.Block, error) {
	resp, err := deliverCli.Recv()
	if err != nil {
		return nil, fmt.Errorf("deliver recv: %w", err)
	}
	switch t := resp.Type.(type) {
	case *orderer.DeliverResponse_Status:
		return nil, fmt.Errorf("can't read the block: %s", t.Status.String())
	case *orderer.DeliverResponse_Block:
		if resp, err := deliverCli.Recv(); err != nil {
			return t.Block, err
		} else if status := resp.GetStatus(); status != common.Status_SUCCESS {
			return nil, fmt.Errorf("non-success status: %s", resp.GetStatus().String())
		}
		return t.Block, nil
	default:
		return nil, errors.Errorf("response error: unknown type %T", t)
	}
}

// OrdererBroadcast broadcasts a common.Envelope to multiple orderer nodes and
// waits for a quorum of successful responses.
func OrdererBroadcast(ctx context.Context, l *zap.Logger, env *common.Envelope, quorum int, ordererClients ...orderer.AtomicBroadcastClient) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	resChan := make(chan error, len(ordererClients))

	for _, ord := range ordererClients {
		ord := ord
		go func(ctx context.Context, ord orderer.AtomicBroadcastClient) {
			cli, err := ord.Broadcast(ctx)
			if err != nil {
				resChan <- fmt.Errorf("get broadcast stream: %w", err)
				return
			}
			if err = cli.Send(env); err != nil {
				resChan <- fmt.Errorf("send broadcast: %w", err)
				return
			}
			resp, err := cli.Recv()
			if err != nil {
				resChan <- fmt.Errorf("client recv: %w", err)
				return
			}
			if resp.Status != common.Status_SUCCESS {
				resChan <- fmt.Errorf("unexpected orderer status: %s with info: %s", resp.Status.String(), resp.Info)
				return
			}
			resChan <- nil
		}(ctx, ord)
	}

	var (
		err       error
		goodCount int
	)

	nOrders := len(ordererClients)

	for i := 0; i < nOrders; i++ {
		res := <-resChan
		if res != nil {
			err = res
			l.Debug("got orderer error response", zap.Error(res))
			continue
		}

		goodCount++
		if goodCount >= quorum {
			l.Debug("quorum is reached", zap.Int("good", goodCount), zap.Int("quorum", quorum))
			return nil
		}
	}

	if goodCount > 0 {
		l.Debug("quorum is not reached but there is success", zap.Int("good", goodCount), zap.Int("quorum", quorum))
		return nil
	}

	l.Error("quorum is not reached", zap.Int("good", goodCount), zap.Int("quorum", quorum))
	return err
}
