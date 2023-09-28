package plane

import (
	"context"

	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/lifecycle"
	lifecycle2 "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *srv) LifecycleApproved(ctx context.Context, req *proto.LifecycleApprovedRequest) (*proto.LifecycleApprovedResponse, error) {
	logger := s.logger.With(
		zap.String("channel", req.ChannelName),
		zap.String("chaincode", req.ChaincodeName),
	)

	// get random endorser instance
	logger.Debug("get random endorser instance")
	endCli, err := s.peerPool.GetRandomEndorser(ctx, s.mspID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get endorser: %v", err)
	}
	lcCli := lifecycle.NewClient(endCli, s.id)
	// search chaincode in committed
	logger.Debug("query approved chaincode definition on channel")
	approved, err := lcCli.QueryApproved(ctx, req.ChannelName, req.ChaincodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query approved: %v", err)
	}

	var pkgID string
	if val, ok := approved.Source.Type.(*lifecycle2.ChaincodeSource_LocalPackage); ok {
		pkgID = val.LocalPackage.PackageId
	} else {
		logger.Warn("get chaincode package failed cause unknown chaincode type", zap.Reflect("type", approved.Source.Type))
	}

	return &proto.LifecycleApprovedResponse{
		Chaincode: &proto.LifecycleChaincode{
			Name:         req.ChaincodeName,
			Sequence:     approved.Sequence,
			Version:      approved.Version,
			InitRequired: approved.InitRequired,
		},
		PackageId: pkgID,
	}, nil
}
