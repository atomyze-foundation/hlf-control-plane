package util

import (
	"context"
	"fmt"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"golang.org/x/sync/errgroup"
)

func EndorsePeers(ctx context.Context, signedProp *pb.SignedProposal, endCli ...pb.EndorserClient) ([]*pb.ProposalResponse, error) {
	g, ctx := errgroup.WithContext(ctx)
	responseChan := make(chan *pb.ProposalResponse)
	for _, cli := range endCli {
		cli := cli
		g.Go(func() error {
			resp, err := cli.ProcessProposal(ctx, signedProp)
			if err != nil {
				return fmt.Errorf("process proposal: %w", err)
			}
			responseChan <- resp
			return nil
		})
	}

	responses := make([]*pb.ProposalResponse, 0, len(endCli))
	doneChan := make(chan struct{})

	go func() {
		for resp := range responseChan {
			responses = append(responses, resp)
		}
		doneChan <- struct{}{}
	}()

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("wait: %w", err)
	}
	close(responseChan)
	<-doneChan

	return responses, nil
}
