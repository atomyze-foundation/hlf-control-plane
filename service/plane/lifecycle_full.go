package plane

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/orderer"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/peer"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/cscc"
	"github.com/atomyze-foundation/hlf-control-plane/system/lifecycle"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pp "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/policydsl"
	hlfUtil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protoutil"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultTryCount = 3
	defaultSleep    = time.Millisecond * 50
)

func (s *srv) LifecycleFull(ctx context.Context, req *pb.LifecycleFullRequest) (*pb.LifecycleFullResponse, error) { //nolint:funlen,gocognit,gocyclo
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

	policyBytes, err := proto.Marshal(&pp.ApplicationPolicy{
		Type: &pp.ApplicationPolicy_SignaturePolicy{
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
	// search chaincode in committed
	logger.Debug("query committed chaincode on channel")

	committed, err := lcCli.QueryCommitted(ctx, req.ChannelName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query committed: %v", err)
	}

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

	initRequired := req.InitRequired
	if !req.InitRequired {
		initRequired = len(req.InitArgs) > 0
	}

	logger.Debug("init required flag value", zap.Bool("init", initRequired))

	// do nothing if chaincode is committed
	if s.foundCommitted(req, initRequired, committed) { //nolint:nestif
		logger.Debug("found committed chaincode definition, nothing to do")
		if initRequired {
			peers, orderers, consType, err := s.getPeersAndOrderersFromConf(ctx, endCli, req.ChannelName)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "get peers and orderers from channel config: %v", err)
			}

			peersDis, err := s.discCli.GetEndorsers(ctx, req.ChannelName, req.ChaincodeName)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "get peers from discovery: %v", err)
			}

			endorsers, err := GetEndorsersFromDiscovery(ctx, s.peerPool, peers, peersDis)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "get peers from discovery: %v", err)
			}

			if err = s.initChaincode(ctx, req.ChannelName, req.ChaincodeName, req.InitArgs, endorsers, orderers, consType); err != nil {
				return nil, status.Errorf(codes.Internal, "init chaincode: %v", err)
			}
		}
		return &pb.LifecycleFullResponse{
			Committed: true,
			Approvals: nil,
		}, nil
	}

	// check chaincode approval and approve if it's not
	logger.Debug("query approved chaincode definition")
	approved, err := lcCli.QueryApproved(ctx, req.ChannelName, req.ChaincodeName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query approved: %v", err)
	}

	// calculate chaincode sequence based con current approved and committed definition
	sequence := s.calculateSequence(req, approved, committed)

	// if chaincode is not approved
	logger.Debug("check chaincode is approved on channel")
	if !s.isApproved(req, policyBytes, approved, initRequired) { //nolint:nestif
		logger.Debug("chaincode is not approved")
		// increase sequence number
		logger.Debug("current chaincode sequence", zap.Int64("sequence", sequence), zap.String("chaincode", req.ChaincodeName))
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
		endorsers := make([]pp.EndorserClient, 0, len(peers))
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
			InitRequired:    initRequired,
			SignaturePolicy: policyBytes,
			Sequence:        sequence,
			Version:         req.Version,
		}))
		txID, env, err := lcCli.ApproveForMyOrg(ctx, req.ChannelName, &lifecycle.ApproveRequest{
			Name:            req.ChaincodeName,
			PackageID:       packageID,
			InitRequired:    initRequired,
			SignaturePolicy: policyBytes,
			Sequence:        sequence,
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
	}
	logger.Debug("chaincode is approved")

	// check committees of chaincode
	logger.Debug("check approval status for all organizations")
	readyToCommit, approvals, err := s.isReadyToCommit(ctx, endCli, req.ChannelName, &lb.CheckCommitReadinessArgs{
		Sequence:            sequence,
		Name:                req.ChaincodeName,
		Version:             req.Version,
		InitRequired:        initRequired,
		ValidationParameter: policyBytes,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check ready to commit: %v", err)
	}

	// if chaincode isn't ready to commit return current status
	if !req.CommitForce && !readyToCommit {
		logger.Debug("chaincode isn't ready to commit", zap.Reflect("approvals", approvals))
		return &pb.LifecycleFullResponse{
			Committed: false,
			Approvals: approvals,
		}, nil
	}

	logger.Debug("chaincode is ready to commit", zap.Reflect("approvals", approvals))
	return s.commitAndInit(ctx, endCli, req, initRequired, sequence, policyBytes)
}

// foundCommitted - checks request chaincode is already committed
func (s *srv) foundCommitted(req *pb.LifecycleFullRequest, initRequired bool, committed []*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition) bool {
	var found bool
	for _, cc := range committed {
		if cc.Name == req.ChaincodeName && cc.Version == req.Version && cc.InitRequired == initRequired {
			found = true
			break
		}
	}
	return found
}

// isApproved - checks request chaincode is already approved
func (s *srv) isApproved(req *pb.LifecycleFullRequest, policyBytes []byte, approved *lb.QueryApprovedChaincodeDefinitionResult, initRequired bool) bool {
	return req.Version == approved.Version && approved.InitRequired == initRequired && bytes.Equal(policyBytes, approved.ValidationParameter)
}

// isReadyToCommit - checks commit readiness of chaincode: current approvals and possibility to commit
func (s *srv) isReadyToCommit(ctx context.Context, endCli pp.EndorserClient, channelName string, req *lb.CheckCommitReadinessArgs) (bool, map[string]bool, error) {
	cr, err := lifecycle.NewClient(endCli, s.id).CheckCommitReadiness(ctx, channelName, req)
	if err != nil {
		return false, nil, fmt.Errorf("CheckCommitReadiness: %w", err)
	}

	approved := 0
	for _, app := range cr.Approvals {
		if app {
			approved++
		}
	}

	if len(cr.Approvals) == approved {
		return true, cr.Approvals, nil
	}

	return false, cr.Approvals, nil
}

// getVersionFromPackageID - default version by name chaincode package
func (s *srv) getVersionFromPackageID(pkgID string) string {
	return strings.Split(pkgID, ":")[0]
}

// calculateSequence - calculate future chaincode sequence
func (s *srv) calculateSequence(req *pb.LifecycleFullRequest, approved *lb.QueryApprovedChaincodeDefinitionResult, committed []*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition) int64 {
	// if chaincode is approved and already committed we should increase sequence
	for _, cc := range committed {
		if cc.Name == req.ChaincodeName && cc.Sequence == approved.Sequence {
			return cc.Sequence + 1
		}
	}
	// check approved at first time
	if approved.Sequence == 0 {
		return 1
	}
	// otherwise, return current approved version cause chaincode is not committed
	return approved.Sequence
}

// getPackageIDByLabel - get package id of installed chaincode by it's label
func (s *srv) getPackageIDByLabel(ctx context.Context, endCli pp.EndorserClient, label string) (string, error) {
	installed, err := lifecycle.NewClient(endCli, s.id).QueryInstalled(ctx)
	if err != nil {
		return "", fmt.Errorf("query installed: %w", err)
	}

	for _, cc := range installed {
		if cc.Label == label {
			return cc.PackageID, nil
		}
	}
	return "", fmt.Errorf("package with label %s not found", label)
}

func (s *srv) sendProposalToOrderers(ctx context.Context, env *common.Envelope, consType pb.ConsensusType, orderers []*pb.Orderer) error {
	switch consType {
	case pb.ConsensusType_CONSENSUS_TYPE_RAFT:
		ordRnd := orderers[rand.Intn(len(orderers)-1)]
		ordCli, err := s.ordPool.Get(&orderer.Orderer{
			Host:         ordRnd.Host,
			Port:         ordRnd.Port,
			Certificates: ordRnd.CaCerts,
		})
		if err != nil {
			return fmt.Errorf("get orderer from pool: %w", err)
		}
		return util.OrdererBroadcast(ctx, s.logger, env, 1, ordCli)
	case pb.ConsensusType_CONSENSUS_TYPE_BFT:
		ordClis := make([]ab.AtomicBroadcastClient, 0, len(orderers))
		for _, ord := range orderers {
			ordCli, err := s.ordPool.Get(&orderer.Orderer{
				Host:         ord.Host,
				Port:         ord.Port,
				Certificates: ord.CaCerts,
			})
			if err != nil {
				return fmt.Errorf("get orderer from pool: %w", err)
			}
			ordClis = append(ordClis, ordCli)
		}
		return util.OrdererBroadcast(ctx, s.logger, env, s.quorumBft(len(ordClis)), ordClis...)
	default:
		return fmt.Errorf("unknown consensus type: %s", consType.String())
	}
}

func (s *srv) quorumBft(n int) (q int) {
	f := (n - 1) / 3                                        //nolint:gomnd
	q = int(math.Ceil((float64(n) + float64(f) + 1) / 2.0)) //nolint:gomnd
	return
}

// commitAndInit - commits chaincode definition on channel and endorse peer for chaincode init
func (s *srv) commitAndInit( //nolint:funlen,gocognit
	ctx context.Context,
	endCli pp.EndorserClient,
	req *pb.LifecycleFullRequest,
	initRequired bool,
	seq int64,
	policyBytes []byte,
) (*pb.LifecycleFullResponse, error) {
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
		Sequence:            seq,
		Name:                req.ChaincodeName,
		Version:             req.Version,
		ValidationParameter: policyBytes,
		InitRequired:        initRequired,
	}), zap.Int("endorsers_len", len(endorsers)))

	var (
		txID string
		env  *common.Envelope
	)
m1:
	for i := 0; i < defaultTryCount; i++ {
		txID, env, err = lifecycle.NewClient(endCli, s.id).Commit(ctx, req.ChannelName, &lb.CommitChaincodeDefinitionArgs{
			Sequence:            seq,
			Name:                req.ChaincodeName,
			Version:             req.Version,
			ValidationParameter: policyBytes,
			InitRequired:        initRequired,
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
	var txCode pp.TxValidationCode
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
		if txCode != pp.TxValidationCode_VALID {
			return nil, status.Errorf(codes.Internal, "tx validation failed with code: %s", txCode.String())
		}

	case <-ctx.Done():
		return nil, status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}

	// init chaincode if init arguments are provided
	if initRequired {
		peersDis, err = s.discCli.GetEndorsers(ctx, req.ChannelName, req.ChaincodeName)
		if err != nil {
			return nil, fmt.Errorf("get peers from discovery: %w", err)
		}

		endorsers, err = GetEndorsersFromDiscovery(ctx, s.peerPool, peers, peersDis)
		if err != nil {
			return nil, fmt.Errorf("get peers from discovery: %w", err)
		}

		s.logger.Debug("call init chaincode cause initArgs is not nil")
		if err = s.initChaincode(ctx, req.ChannelName, req.ChaincodeName, req.InitArgs, endorsers, orderers, consType); err != nil {
			return nil, status.Errorf(codes.Internal, "init chaincode: %v", err)
		}
	}

	return &pb.LifecycleFullResponse{
		Committed: true,
		Approvals: nil,
	}, nil
}

func (s *srv) initChaincode(ctx context.Context, channelName string, ccName string, initArgs []string, endorsers []pp.EndorserClient, orderers []*pb.Orderer, consType pb.ConsensusType) error { //nolint:funlen
	// create invocation spec
	cis := &pp.ChaincodeInvocationSpec{
		ChaincodeSpec: &pp.ChaincodeSpec{
			Type:        pp.ChaincodeSpec_GOLANG,
			ChaincodeId: &pp.ChaincodeID{Name: ccName},
			Input:       &pp.ChaincodeInput{IsInit: true, Args: hlfUtil.ToChaincodeArgs(initArgs...)},
		},
	}
	// serialize signer
	signerSerialized, err := s.id.Serialize()
	if err != nil {
		return fmt.Errorf("serialize identity: %w", err)
	}
	// create endorsement proposal
	prop, txID, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, channelName, cis, signerSerialized)
	if err != nil {
		return fmt.Errorf("crailed to eate ChaincodeInvocationSpec proposal: %w", err)
	}
	// sign proposal
	signedProp, err := util.SignProposal(prop, s.id)
	if err != nil {
		return fmt.Errorf("sign proposal: %w", err)
	}
	// endorse signed proposal on peers
	responses, err := util.EndorsePeers(ctx, signedProp, endorsers...)
	if err != nil {
		return fmt.Errorf("endorse peers: %w", err)
	}
	// check chaincode is already initialized
	if responses[0].Response.Status != http.StatusOK {
		if strings.Contains(responses[0].Response.Message, "is already initialized but called as init") {
			return nil
		}
	}

	// create transaction to orderer based on proposal responses
	env, err := protoutil.CreateSignedTx(prop, s.id, responses...)
	if err != nil {
		return fmt.Errorf("create signed tx: %w", err)
	}

	res := make(chan error)
	ready := make(chan struct{})
	ctx = util.NewContext(ctx, ready)
	go func() {
		// wait tx validation on peers
		select {
		case res <- s.initChaincodeValidCheck(ctx, channelName, txID):
		case <-ctx.Done():
		}
	}()

	select {
	case <-ready:
	case <-ctx.Done():
		return status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}

	// send transaction to orderer
	if err = s.sendProposalToOrderers(ctx, env, consType, orderers); err != nil {
		return fmt.Errorf("send proposal to orderer: %w", err)
	}

	select {
	case err = <-res:
		return err
	case <-ctx.Done():
		return status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}
}

func (s *srv) initChaincodeValidCheck(ctx context.Context, channelName string, txID string) error {
	// wait block validation on peer
	txCode, err := s.dCli.SubscribeTx(ctx, channelName, txID)
	if err != nil {
		return fmt.Errorf("subscribe tx: %w", err)
	}
	if txCode != pp.TxValidationCode_VALID {
		return fmt.Errorf("tx validation failed with code: %s", txCode.String())
	}
	return nil
}

func (s *srv) getPeersAndOrderersFromConf(ctx context.Context, endCli pp.EndorserClient, channelName string) ([]*peer.Peer, []*pb.Orderer, pb.ConsensusType, error) {
	// get current channel config
	s.logger.Debug("get channel config", zap.String("channel", channelName))
	conf, err := cscc.NewClient(endCli, s.id).GetChannelConfig(ctx, channelName)
	if err != nil {
		s.logger.Error("get channel config error", zap.Error(err))
		return nil, nil, 0, status.Errorf(codes.Internal, "get channel config: %v", err)
	}
	// get peers endpoint from anchor peer configuration
	peers, err := util.GetPeerConfig(conf)
	if err != nil {
		s.logger.Error("get peers from channel config error", zap.Error(err))
		return nil, nil, 0, status.Errorf(codes.Internal, "get peers from config: %v", err)
	}

	orderers, consType, err := util.GetOrdererConfig(conf)
	if err != nil {
		return nil, nil, 0, status.Errorf(codes.Internal, "get orderer config: %v", err)
	}
	return peers, orderers, consType, nil
}

func GetEndorsersFromDiscovery(ctx context.Context, peerPool peer.Pool, peersCfg []*peer.Peer, peersDis []*pb.DiscoveryPeer) ([]pp.EndorserClient, error) {
	peers := make([]*peer.Peer, 0, len(peersDis))

m1:
	for _, p := range peersDis {
		for _, pCfg := range peersCfg {
			if pCfg.Host == p.Host &&
				pCfg.Port == p.Port {
				peers = append(peers, pCfg)
				continue m1
			}
		}
		peers = append(peers, &peer.Peer{
			Host:  p.Host,
			Port:  p.Port,
			MspID: p.MspId,
		})
	}

	// get endorsers from peer pool
	endorsers := make([]pp.EndorserClient, 0, len(peers))
	for _, p := range peers {
		endCli, err := peerPool.GetEndorser(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("get endorser: %w", err)
		}
		endorsers = append(endorsers, endCli)
	}

	return endorsers, nil
}
