package plane

import (
	"context"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/cscc"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *srv) ConfigAnchorList(ctx context.Context, req *proto.ConfigAnchorListRequest) (*proto.ConfigAnchorListResponse, error) {
	logger := s.logger.With(zap.String("channel", req.ChannelName))
	logger.Debug("get channel config", zap.String("channel", req.ChannelName))
	endCli, err := s.peerPool.GetRandomEndorser(ctx, s.mspID)
	if err != nil {
		logger.Error("get endorser failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get endorser failed: %v", err)
	}
	conf, err := cscc.NewClient(endCli, s.id).GetChannelConfig(ctx, req.ChannelName)
	if err != nil {
		logger.Error("get config failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get channel config: %v", err)
	}
	peers, err := util.GetAnchorPeerConfig(conf, s.mspID)
	if err != nil {
		logger.Error("get anchor peer config failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get anchor peer config: %v", err)
	}

	return &proto.ConfigAnchorListResponse{Result: peers}, nil
}
