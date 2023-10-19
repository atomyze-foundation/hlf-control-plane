package plane

import (
	"context"

	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *srv) DiscoveryConfig(ctx context.Context, req *pb.DiscoveryConfigRequest) (*pb.DiscoveryConfigResponse, error) {
	config, err := s.discCli.GetConfig(ctx, req.ChannelName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get discovery config: %v", err)
	}
	return &pb.DiscoveryConfigResponse{Result: config}, nil
}
