package plane

import (
	"context"
	"fmt"
	"sync"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/orderer"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/cscc"
	"github.com/hyperledger/fabric-protos-go/common"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *srv) ChannelJoin(ctx context.Context, req *pb.ChannelJoinRequest) (*pb.ChannelJoinResponse, error) {
	// get orderer client
	ordCli, err := s.ordPool.Get(&orderer.Orderer{
		Host: req.Orderer.Host,
		Port: req.Orderer.Port,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get orderer: %v", err)
	}
	// get envelope for orderer deliver blocks
	env, err := util.GetSeekOldestEnvelope(req.ChannelName, s.id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get seek envelope: %v", err)
	}

	// get deliver client and send envelope
	deliverCli, err := ordCli.Deliver(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get orderer deliver client: %v", err)
	}

	defer func() {
		if err = deliverCli.CloseSend(); err != nil {
			s.logger.Error("close deliver stream error", zap.String("orderer", req.Orderer.Host), zap.Error(err))
		}
	}()

	if err = deliverCli.Send(env); err != nil {
		return nil, status.Errorf(codes.Internal, "deliver send: %v", err)
	}

	// get block from deliver stream
	block, err := util.GetBlockFromDeliverClient(deliverCli)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get block: %v", err)
	}

	return s.processBlockJoin(ctx, req.ChannelName, block)
}

func (s *srv) processBlockJoin(ctx context.Context, channelName string, b *common.Block) (*pb.ChannelJoinResponse, error) {
	var wg sync.WaitGroup
	resChan := make(chan *pb.ChannelJoinResponse_PeerResult)

	for _, p := range s.localPeers {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			endCli, err := s.peerPool.GetEndorser(ctx, p)
			if err != nil {
				resChan <- &pb.ChannelJoinResponse_PeerResult{
					Peer:   p.String(),
					Result: &pb.ChannelJoinResponse_PeerResult_Err{Err: fmt.Sprintf("get endorser: %s", err)},
				}
				return
			}
			s.processPeerJoin(ctx, p.String(), cscc.NewClient(endCli, s.id), channelName, b, resChan)
		}()
	}

	doneChan := make(chan struct{})
	result := make([]*pb.ChannelJoinResponse_PeerResult, 0)
	go func() {
		for res := range resChan {
			result = append(result, res)
		}
		doneChan <- struct{}{}
	}()
	wg.Wait()
	close(resChan)
	<-doneChan

	return &pb.ChannelJoinResponse{Result: result}, nil
}

func (s *srv) processPeerJoin(ctx context.Context, peerName string, cli cscc.Client, channelName string, block *common.Block, resChan chan<- *pb.ChannelJoinResponse_PeerResult) {
	res := &pb.ChannelJoinResponse_PeerResult{Peer: peerName}
	defer func() {
		resChan <- res
	}()
	// search proposed channel to join in already joined
	joined, err := cli.GetChannels(ctx)
	if err != nil {
		res.Result = &pb.ChannelJoinResponse_PeerResult_Err{Err: fmt.Sprintf("get joined channels: %v", err)}
		return
	}
	// search channel is already joined
	var found bool
	for _, ch := range joined {
		if ch == channelName {
			found = true
		}
	}
	// return found flag if peer is already joined
	if found {
		res.Result = &pb.ChannelJoinResponse_PeerResult_Existed{Existed: true}
		return
	}
	// join channel
	if err = cli.JoinChain(ctx, block); err != nil {
		res.Result = &pb.ChannelJoinResponse_PeerResult_Err{Err: fmt.Sprintf("join chain: %v", err)}
		return
	}
	res.Result = &pb.ChannelJoinResponse_PeerResult_Existed{Existed: false}
}
