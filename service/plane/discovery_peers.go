package plane

import (
	"context"

	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *srv) DiscoveryPeers(ctx context.Context, req *pb.DiscoveryPeersRequest) (*pb.DiscoveryPeersResponse, error) {
	peers, err := s.discCli.GetPeers(ctx, req.ChannelName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get peers: %v", err)
	}
	return &pb.DiscoveryPeersResponse{Result: peers}, nil
}
