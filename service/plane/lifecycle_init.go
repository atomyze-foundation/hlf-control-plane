package plane

import (
	"context"
	"fmt"

	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *srv) LifecycleInit(
	ctx context.Context,
	req *pb.LifecycleInitRequest,
) (*emptypb.Empty, error) {
	logger := s.logger.With(
		zap.String("chaincode", req.ChaincodeName),
		zap.String("channel", req.ChannelName),
	)

	// get random endorser instance
	logger.Debug("get random endorser instance")
	endCli, err := s.peerPool.GetRandomEndorser(ctx, s.mspID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get endorser: %v", err)
	}

	peers, orderers, consType, err := s.getPeersAndOrderersFromConf(ctx, endCli, req.ChannelName)
	if err != nil {
		return nil, fmt.Errorf("get peers and orderers from conf: %w", err)
	}

	peersDis, err := s.discCli.GetEndorsers(ctx, req.ChannelName, req.ChaincodeName)
	if err != nil {
		return nil, fmt.Errorf("get peers from discovery: %w", err)
	}

	endorsers, err := GetEndorsersFromDiscovery(ctx, s.peerPool, peers, peersDis)
	if err != nil {
		return nil, fmt.Errorf("get peers from discovery: %w", err)
	}

	s.logger.Debug("call init chaincode cause initArgs is not nil")
	if err = s.initChaincode(ctx, req.ChannelName, req.ChaincodeName, req.InitArgs, endorsers, orderers, consType); err != nil {
		return nil, status.Errorf(codes.Internal, "init chaincode: %v", err)
	}

	logger.Debug("chaincode is Init")

	return &emptypb.Empty{}, nil
}
