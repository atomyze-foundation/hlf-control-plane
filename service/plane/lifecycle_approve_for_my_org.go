package plane

import (
	"context"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/cscc"
	"github.com/atomyze-foundation/hlf-control-plane/system/lifecycle"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policydsl"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *srv) LifecycleApproveForMyOrg( //nolint:funlen
	ctx context.Context,
	req *pb.LifecycleApproveForMyOrgRequest,
) (*emptypb.Empty, error) {
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

	// get package id from installed chaincodes by chaincode label
	logger.Debug("get package id by it's label")
	packageID, err := s.getPackageIDByLabel(ctx, endCli, req.ChaincodeLabel)
	if err != nil {
		return nil, status.Error(codes.NotFound, "package not found")
	}

	// if version is not provided, get it from chaincode package
	if req.Version == "" {
		req.Version = s.getVersionFromPackageID(packageID)
		logger = logger.With(zap.String("version", req.Version))
		logger.Debug("calculated version for chaincode")
	}

	// fetch current channel config
	logger.Debug("get channel config")
	conf, err := cscc.NewClient(endCli, s.id).GetChannelConfig(ctx, req.ChannelName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get channel config: %v", err)
	}

	// get peers endpoint from anchor peer configuration
	logger.Debug("get peers from channel config", zap.String("mspId", s.mspID))
	peers, err := util.GetPeerConfig(conf, s.mspID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get peers from config: %v", err)
	}

	logger.Debug("list of peer to endorse", zap.Reflect("peers", peers))
	endorsers := make([]peer.EndorserClient, 0, len(peers))
	for _, p := range peers {
		endCli, err := s.peerPool.GetEndorser(ctx, p)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "get endorser: %v", err)
		}
		endorsers = append(endorsers, endCli)
	}

	// endorse lifecycle chaincode with new chaincode approval and create envelope for orderer
	logger.Debug("call lifecycle approveformyorg", zap.Reflect("request", lifecycle.ApproveRequest{
		Name:            req.ChaincodeName,
		PackageID:       packageID,
		InitRequired:    req.InitRequired,
		SignaturePolicy: policyBytes,
		Sequence:        req.Sequence,
		Version:         req.Version,
	}))
	txID, env, err := lcCli.ApproveForMyOrg(ctx, req.ChannelName, &lifecycle.ApproveRequest{
		Name:            req.ChaincodeName,
		PackageID:       packageID,
		InitRequired:    req.InitRequired,
		SignaturePolicy: policyBytes,
		Sequence:        req.Sequence,
		Version:         req.Version,
	}, endorsers...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ApproveForMyOrg: %v", err)
	}
	logger.Debug("approveformyorg success", zap.String("txId", txID))

	// get orderer and consensus type from channel config
	// if Raft - envelope will be sent to random orderer
	// BFT - envelope will be sent to all orderers
	logger.Debug("get orderer config")
	orderers, consType, err := util.GetOrdererConfig(conf)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get orderer config: %v", err)
	}

	res := make(chan error)
	ready := make(chan struct{})
	ctx = util.NewContext(ctx, ready)
	go func() {
		// wait tx validation on peers
		logger.Debug("wait for peer tx event", zap.String("channel", req.ChannelName), zap.String("txId", txID))
		select {
		case res <- s.dCli.SubscribeTxAll(ctx, req.ChannelName, txID):
		case <-ctx.Done():
		}
	}()

	select {
	case <-ready:
	case <-ctx.Done():
		return nil, status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}

	logger.Debug("send envelope to orderers")
	if err = s.sendProposalToOrderers(ctx, env, consType, orderers); err != nil {
		return nil, status.Errorf(codes.Internal, "send proposal to orderer: %v", err)
	}

	select {
	case err = <-res:
		if err != nil {
			return nil, status.Errorf(codes.Internal, "tx validation: %v", err)
		}
	case <-ctx.Done():
		return nil, status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}

	logger.Debug("chaincode is approved")

	return &emptypb.Empty{}, nil
}
