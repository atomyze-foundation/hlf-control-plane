package plane

import (
	"context"

	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/lifecycle"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *srv) LifecycleStatus(ctx context.Context, req *proto.LifecycleStatusRequest) (*proto.LifecycleStatusResponse, error) {
	logger := s.logger.With(
		zap.String("channel", req.ChannelName),
	)

	// get random endorser instance
	logger.Debug("get random endorser instance")
	endCli, err := s.peerPool.GetRandomEndorser(ctx, s.mspID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get endorser: %v", err)
	}
	lcCli := lifecycle.NewClient(endCli, s.id)
	// search chaincode in committed
	logger.Debug("query committed chaincode on channel")

	committed, err := lcCli.QueryCommitted(ctx, req.ChannelName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query committed: %v", err)
	}

	chaincodes := make([]*proto.LifecycleChaincode, 0, len(committed))
	for _, cc := range committed {
		chaincodes = append(chaincodes, &proto.LifecycleChaincode{
			Name:         cc.Name,
			Sequence:     cc.Sequence,
			Version:      cc.Version,
			InitRequired: cc.InitRequired,
		})
	}

	return &proto.LifecycleStatusResponse{Chaincodes: chaincodes}, nil
}
