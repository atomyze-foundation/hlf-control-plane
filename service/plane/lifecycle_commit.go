package plane

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/lifecycle"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/policydsl"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *srv) LifecycleCommit( //nolint:funlen
	ctx context.Context,
	req *pb.LifecycleCommitRequest,
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

	peers, orderers, consType, err := s.getPeersAndOrderersFromConf(ctx, endCli, req.ChannelName)
	if err != nil {
		return nil, fmt.Errorf("get peers and orderers from conf: %w", err)
	}

	peersDis, err := s.discCli.GetEndorsers(ctx, req.ChannelName, lifecycle.CcName)
	if err != nil {
		return nil, fmt.Errorf("get peers from discovery: %w", err)
	}

	endorsers, err := GetEndorsersFromDiscovery(ctx, s.peerPool, peers, peersDis)
	if err != nil {
		return nil, fmt.Errorf("get peers from discovery: %w", err)
	}

	// commit chaincode definition using presented endorsers
	s.logger.Debug("commit chaincode definition", zap.Reflect("args", lb.CommitChaincodeDefinitionArgs{
		Sequence:            req.Sequence,
		Name:                req.ChaincodeName,
		Version:             req.Version,
		ValidationParameter: policyBytes,
		InitRequired:        req.InitRequired,
	}), zap.Int("endorsers_len", len(endorsers)))

	var (
		txID string
		env  *common.Envelope
	)
m1:
	for i := 0; i < defaultTryCount; i++ {
		txID, env, err = lifecycle.NewClient(endCli, s.id).Commit(ctx, req.ChannelName, &lb.CommitChaincodeDefinitionArgs{
			Sequence:            req.Sequence,
			Name:                req.ChaincodeName,
			Version:             req.Version,
			ValidationParameter: policyBytes,
			InitRequired:        req.InitRequired,
		}, endorsers...)
		if err == nil ||
			!strings.Contains(err.Error(), "ProposalResponsePayloads do not match (base64)") {
			break
		}

		select {
		case <-time.After(defaultSleep):
		case <-ctx.Done():
			break m1
		}

		peersDis, err = s.discCli.GetEndorsers(ctx, req.ChannelName, lifecycle.CcName)
		if err != nil {
			return nil, fmt.Errorf("get peers from discovery: %w", err)
		}

		endorsers, err = GetEndorsersFromDiscovery(ctx, s.peerPool, peers, peersDis)
		if err != nil {
			return nil, fmt.Errorf("get peers from discovery: %w", err)
		}
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "commit: %v", err)
	}

	s.logger.Debug("chaincode committed, now subscribe on events")
	// get orderer and consensus type from channel config
	// if Raft - envelope will be sent to random orderer
	// BFT - envelope will be sent to all orderers

	res := make(chan error)
	ready := make(chan struct{})
	ctx = util.NewContext(ctx, ready)
	var txCode peer.TxValidationCode
	go func() {
		// wait tx validation on peers
		txCode, err = s.dCli.SubscribeTx(ctx, req.ChannelName, txID)
		select {
		case res <- err:
		case <-ctx.Done():
		}
	}()

	select {
	case <-ready:
	case <-ctx.Done():
		return nil, status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}

	if err = s.sendProposalToOrderers(ctx, env, consType, orderers); err != nil {
		return nil, status.Errorf(codes.Internal, "send proposal to orderer: %v", err)
	}

	select {
	case err = <-res:
		if err != nil {
			return nil, status.Errorf(codes.Internal, "tx validation: %v", err)
		}
		// check tx by validation code
		if txCode != peer.TxValidationCode_VALID {
			return nil, status.Errorf(codes.Internal, "tx validation failed with code: %s", txCode.String())
		}

	case <-ctx.Done():
		return nil, status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}

	logger.Debug("chaincode is commit")

	return &emptypb.Empty{}, nil
}
