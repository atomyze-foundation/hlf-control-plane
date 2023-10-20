package plane

import (
	"context"
	"fmt"
	"sort"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/peer"
	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/cscc"
	"github.com/atomyze-foundation/hlf-control-plane/system/qscc"
	pp "github.com/hyperledger/fabric-protos-go/peer"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"
)

type getChannelsResult struct {
	peer     string
	channels map[string]getChannelsChannelInfo
}

type getChannelsChannelInfo struct {
	blockNumber   uint64
	blockHash     []byte
	prevBlockHash []byte
}

func (s *srv) ChannelJoined(ctx context.Context, _ *emptypb.Empty) (*pb.ChannelJoinedResponse, error) {
	g, ctx := errgroup.WithContext(ctx)
	resultChan := make(chan getChannelsResult)
	for _, p := range s.localPeers {
		p := p
		endCli, err := s.peerPool.GetEndorser(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("get endorser: %w", err)
		}
		g.Go(s.getChannelsOfPeer(ctx, endCli, p, resultChan))
	}

	doneChan := make(chan struct{})
	waitChan := make(chan struct{})
	go func() {
		<-waitChan
		close(resultChan)
	}()

	results := make([]*pb.ChannelJoinedResponse_Result, 0)
	go s.getChannelsResults(resultChan, &results, doneChan)

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("wait error: %w", err)
	}

	waitChan <- struct{}{}
	<-doneChan

	return &pb.ChannelJoinedResponse{Result: results}, nil
}

func (s *srv) getChannelsOfPeer(ctx context.Context, endCli pp.EndorserClient, p *peer.Peer, results chan<- getChannelsResult) func() error {
	return func() error {
		cli := cscc.NewClient(endCli, s.id)
		channels, err := cli.GetChannels(ctx)
		if err != nil {
			return fmt.Errorf("lifecycle queryinstalled on peer '%s': %w", p.String(), err)
		}

		chMap := make(map[string]getChannelsChannelInfo)
		for _, ch := range channels {
			chainInfo, err := qscc.NewClient(endCli, s.id).GetChainInfo(ctx, ch)
			if err != nil {
				s.logger.Error("getChainInfo error", zap.Error(err), zap.String("peer", p.String()))
				chMap[ch] = getChannelsChannelInfo{blockHash: nil, blockNumber: 0, prevBlockHash: nil}
			} else {
				chMap[ch] = getChannelsChannelInfo{
					blockNumber:   chainInfo.Height,
					blockHash:     chainInfo.CurrentBlockHash,
					prevBlockHash: chainInfo.PreviousBlockHash,
				}
			}
		}

		results <- getChannelsResult{channels: chMap, peer: p.String()}
		return nil
	}
}

func (s *srv) getChannelsResults(resultChan <-chan getChannelsResult, results *[]*pb.ChannelJoinedResponse_Result, doneChan chan<- struct{}) {
	ccMap := make(map[string]map[string]getChannelsChannelInfo)
	for res := range resultChan {
		for cc, chInfo := range res.channels {
			if v, ok := ccMap[cc]; ok {
				v[res.peer] = chInfo
				ccMap[cc] = v
			} else {
				v := make(map[string]getChannelsChannelInfo)
				v[res.peer] = chInfo
				ccMap[cc] = v
			}
		}
	}

	for cc, peerMap := range ccMap {
		peerInfo := make([]*pb.ChannelJoinedResponse_PeerResult, 0, len(peerMap))
		for p, ccInfo := range peerMap {
			peerInfo = append(peerInfo, &pb.ChannelJoinedResponse_PeerResult{
				Peer:          p,
				BlockHash:     ccInfo.blockHash,
				BlockNumber:   ccInfo.blockNumber,
				PrevBlockHash: ccInfo.prevBlockHash,
			})
		}
		sort.Slice(peerInfo, func(i, j int) bool {
			return peerInfo[i].Peer < peerInfo[j].Peer
		})
		*results = append(*results, &pb.ChannelJoinedResponse_Result{
			Name:  cc,
			Peers: peerInfo,
		})
	}
	doneChan <- struct{}{}
}
