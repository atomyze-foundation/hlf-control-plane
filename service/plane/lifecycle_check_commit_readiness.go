package plane

import (
	"context"

	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/lifecycle"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/policydsl"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *srv) LifecycleCheckCommitReadiness(ctx context.Context, req *pb.LifecycleCheckCommitReadinessRequest) (*pb.LifecycleCheckCommitReadinessResponse, error) {
	logger := s.logger.With(
		zap.String("chaincode", req.ChaincodeName),
		zap.String("channel", req.ChannelName),
	)
	// at first parse chaincode endorsement policy
	logger.Debug("parse endorsement policy", zap.String("policy", req.Policy))
	policy, err := policydsl.FromString(req.Policy)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "parse policy: %v", err)
	}

	policyBytes, err := proto.Marshal(&peer.ApplicationPolicy{
		Type: &peer.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: policy,
		},
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "policy marshal: %v", err)
	}
	// get random endorser instance
	logger.Debug("get random endorser instance")
	endCli, err := s.peerPool.GetRandomEndorser(ctx, s.mspID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get endorser: %v", err)
	}
	lcCli := lifecycle.NewClient(endCli, s.id)

	// check committees of chaincode
	logger.Debug("check approval status for all organizations")
	cr, err := lcCli.CheckCommitReadiness(ctx, req.ChannelName, &lb.CheckCommitReadinessArgs{
		Sequence:            req.Sequence,
		Name:                req.ChaincodeName,
		Version:             req.Version,
		InitRequired:        req.InitRequired,
		ValidationParameter: policyBytes,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check approval status for all organizations: %v", err)
	}

	return &pb.LifecycleCheckCommitReadinessResponse{Approvals: cr.GetApprovals()}, nil
}
