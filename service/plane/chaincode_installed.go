package plane

import (
	"context"
	"fmt"
	"sort"
	"sync"

	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/lifecycle"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type installedResult struct {
	peer string
	ccs  []lifecycle.InstalledChaincode
	err  error
}

func (s *srv) ChaincodeInstalled(ctx context.Context, _ *emptypb.Empty) (*pb.ChaincodeInstalledResponse, error) {
	resultChan := make(chan installedResult)
	var wg sync.WaitGroup
	for _, p := range s.localPeers {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			endCli, err := s.peerPool.GetEndorser(ctx, p)
			if err != nil {
				resultChan <- installedResult{peer: p.String(), err: err}
				return
			}
			ccs, err := lifecycle.NewClient(endCli, s.id).QueryInstalled(ctx)
			if err != nil {
				resultChan <- installedResult{peer: p.String(), err: fmt.Errorf("lifecycle queryinstalled on peer '%s': %w", p, err)}
				return
			}
			resultChan <- installedResult{peer: p.String(), ccs: ccs}
		}()
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	result, err := s.getInstalledChaincodesResult(resultChan)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "process result: %v", err)
	}

	return &pb.ChaincodeInstalledResponse{Result: result}, nil
}

func (s *srv) getInstalledChaincodesResult(resultChan <-chan installedResult) ([]*pb.ChaincodeInstalledResponse_Result, error) {
	ccMap := make(map[string]string)
	ccPeerMap := make(map[string][]string)
	for res := range resultChan {
		if res.err != nil {
			return nil, fmt.Errorf("error occurred on peer : %s: %w", res.peer, res.err)
		}
		for _, cc := range res.ccs {
			if _, ok := ccMap[cc.PackageID]; !ok {
				ccMap[cc.PackageID] = cc.Label
			}
			if peers, ok := ccPeerMap[cc.PackageID]; ok {
				ccPeerMap[cc.PackageID] = append(peers, res.peer)
			} else {
				ccPeerMap[cc.PackageID] = []string{res.peer}
			}
		}
	}

	results := make([]*pb.ChaincodeInstalledResponse_Result, 0, len(ccMap))

	for cc, label := range ccMap {
		sortedPeers := ccPeerMap[cc]
		// sort peers for pretty output
		sort.Strings(sortedPeers)
		results = append(results, &pb.ChaincodeInstalledResponse_Result{
			PackageId: cc,
			Label:     label,
			Peers:     sortedPeers,
		})
	}
	return results, nil
}
