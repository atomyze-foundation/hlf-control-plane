package plane

import (
	"context"
	"fmt"

	pb "github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	hlfUtil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/cscc"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const anchorPeerModPolicy = "Admins"

func (s *srv) ConfigAnchorModify(ctx context.Context, req *proto.ConfigAnchorModifyRequest) (*proto.ConfigAnchorModifyResponse, error) {
	logger := s.logger.With(zap.String("channel", req.ChannelName))
	logger.Debug("get channel config", zap.String("channel", req.ChannelName))
	endCli, err := s.peerPool.GetRandomEndorser(ctx, s.mspID)
	if err != nil {
		logger.Error("get endorser failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get endorser failed: %v", err)
	}
	// TODO get channel config from orderer
	/*if req.Orderer != nil {
		s.ordPool.Get(&orderer.Orderer{
			Host:         req.Orderer.Host,
			Port:         req.Orderer.Port,
			Certificates: req.Orderer.CaCerts,
		})
	}*/
	conf, err := cscc.NewClient(endCli, s.id).GetChannelConfig(ctx, req.ChannelName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get channel config: %v", err)
	}
	// get anchor peers from channel config
	logger.Debug("get anchor peer config")
	anchorPeers, err := util.GetAnchorPeerConfig(conf, s.mspID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get anchor peers: %v", err)
	}
	// calculate anchor peer stats
	existedPeers, newPeers, deletedPeers := s.getAnchorPerModifyStats(anchorPeers, req)
	logger.Debug("peer stats calculated", zap.Int("existed", len(existedPeers)), zap.Int("new", len(newPeers)), zap.Int("deleted", len(deletedPeers)))
	// if no update just return information
	if len(newPeers) == 0 && len(deletedPeers) == 0 {
		return &proto.ConfigAnchorModifyResponse{
			New:     newPeers,
			Existed: existedPeers,
			Deleted: deletedPeers,
		}, nil
	}

	// create channel update
	logger.Debug("create anchor peer update")
	updEnv, err := s.createAnchorPeerUpdate(conf, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create update: %v", err)
	}
	logger.Debug("processing anchor peer update")
	// process channel update on orderers
	if err = s.proceedChannelUpdate(ctx, req.ChannelName, conf, updEnv); err != nil {
		return nil, status.Errorf(codes.Internal, "process update: %v", err)
	}

	// return information about peers
	return &proto.ConfigAnchorModifyResponse{
		New:     newPeers,
		Existed: existedPeers,
		Deleted: deletedPeers,
	}, nil
}

func (s *srv) getAnchorPerModifyStats(current []*peer.AnchorPeer, req *proto.ConfigAnchorModifyRequest) (existed []*peer.AnchorPeer, newPeers []*peer.AnchorPeer, deleted []*peer.AnchorPeer) {
	newPeers = append(newPeers, req.Peers...)
loop:
	for _, p := range current {
		for i, ep := range newPeers {
			// if peer exists in request add it to existedPeers
			if ep.Host == p.Host && ep.Port == p.Port {
				existed = append(existed, ep)
				newPeers = append(newPeers[:i], newPeers[i+1:]...)
				continue loop
			}
			// if peer doesn't exist
		}
		// if peer not found in request it's deleted
		deleted = append(deleted, p)
	}
	return
}

func (s *srv) createAnchorPeerUpdate(conf *common.Config, req *proto.ConfigAnchorModifyRequest) (*common.Envelope, error) {
	// copy instance of config
	newConf, ok := pb.Clone(conf).(*common.Config)
	if !ok {
		return nil, fmt.Errorf("smth wrong with type assertion")
	}
	anchorPeers := make([]*peer.AnchorPeer, 0, len(req.Peers))
	for _, p := range req.Peers {
		anchorPeers = append(anchorPeers, &peer.AnchorPeer{Host: p.Host, Port: p.Port})
	}

	applicationGroup, ok := newConf.ChannelGroup.Groups[channelconfig.ApplicationGroupKey]
	if !ok {
		return nil, fmt.Errorf("application group not found")
	}
	for gr, org := range applicationGroup.Groups {
		mspConf, err := util.GetMspConfig(org)
		if err != nil {
			return nil, fmt.Errorf("get msp config: %w", err)
		}
		if mspConf.Name == s.mspID {
			s.logger.Debug("found MspID, change config group", zap.String("group", gr), zap.String("mspID", s.mspID))
			value := new(common.ConfigValue)
			if curValue, ok := org.Values[channelconfig.AnchorPeersKey]; ok {
				s.logger.Debug("found old value for anchor peers", zap.Reflect("anchor_peers", curValue.Value), zap.Uint64("version", curValue.Version))
				value = curValue
				// value.Version = curValue.Version + 1
			} else {
				s.logger.Debug("old value not found, creating new")
				value.ModPolicy = anchorPeerModPolicy
			}
			if value.Value, err = pb.Marshal(&peer.AnchorPeers{AnchorPeers: anchorPeers}); err != nil {
				return nil, fmt.Errorf("marshal anchor peers: %w", err)
			}
			newConf.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups[gr].Values[channelconfig.AnchorPeersKey] = value
			break
		}
	}
	// compute channel config update
	upd, err := util.Compute(conf, newConf)
	if err != nil {
		return nil, fmt.Errorf("compute update: %w", err)
	}

	return s.createUpdateEnvelope(req.ChannelName, upd)
}

func (s *srv) createUpdateEnvelope(channelName string, upd *common.ConfigUpdate) (*common.Envelope, error) {
	// create update envolope
	upd.ChannelId = channelName
	env := &common.ConfigUpdateEnvelope{
		ConfigUpdate: protoutil.MarshalOrPanic(upd),
	}
	// collect envelope signatures
	var err error
	if env.Signatures, err = s.createChannelUpdateSig(env); err != nil {
		return nil, fmt.Errorf("create sig: %w", err)
	}
	// create signed envelope for orderer
	updateTx, err := protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, channelName, s.id, env, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("create signed envelope")
	}
	return updateTx, nil
}

func (s *srv) createChannelUpdateSig(env *common.ConfigUpdateEnvelope) ([]*common.ConfigSignature, error) {
	// create signature header
	sigHeader, err := protoutil.NewSignatureHeader(s.id)
	if err != nil {
		return nil, err
	}
	// sign signature header
	configSig := &common.ConfigSignature{
		SignatureHeader: protoutil.MarshalOrPanic(sigHeader),
	}
	configSig.Signature, err = s.id.Sign(hlfUtil.ConcatenateBytes(configSig.SignatureHeader, env.ConfigUpdate))
	if err != nil {
		return nil, err
	}
	return []*common.ConfigSignature{configSig}, nil
}

func (s *srv) proceedChannelUpdate(ctx context.Context, channelName string, conf *common.Config, env *common.Envelope) error {
	orderers, consType, err := util.GetOrdererConfig(conf)
	if err != nil {
		return fmt.Errorf("get orderer config: %w", err)
	}

	txID, err := util.GetTxIDFromEnvelope(env)
	if err != nil {
		return fmt.Errorf("get txID from envelope: %w", err)
	}

	res := make(chan error)
	ready := make(chan struct{})
	ctx = util.NewContext(ctx, ready)
	go func() {
		// wait tx validation on peers
		s.logger.Debug("wait for peer tx event", zap.String("channel", channelName), zap.String("txId", txID))
		select {
		case res <- s.dCli.SubscribeTxAll(ctx, channelName, txID):
		case <-ctx.Done():
		}
	}()

	select {
	case <-ready:
	case <-ctx.Done():
		return status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}

	if err = s.sendProposalToOrderers(ctx, env, consType, orderers); err != nil {
		return fmt.Errorf("send envelope to ordering: %w", err)
	}

	select {
	case err = <-res:
		return err
	case <-ctx.Done():
		return status.Errorf(codes.Internal, "execute didn't receive block event by context cancel")
	}
}
