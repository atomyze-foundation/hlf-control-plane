package plane

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric-protos-go/orderer/smartbft"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/cscc"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *srv) ConfigOrderingAdd(ctx context.Context, req *pb.ConfigOrderingAddRequest) (*emptypb.Empty, error) {
	logger := s.logger.With(zap.String("channel", req.ChannelName))

	logger.Debug("get channel config", zap.String("channel", req.ChannelName))
	endCli, err := s.peerPool.GetRandomEndorser(ctx, s.mspID)
	if err != nil {
		logger.Error("get endorser failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get endorser failed: %v", err)
	}

	config, err := cscc.NewClient(endCli, s.id).GetChannelConfig(ctx, req.ChannelName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get config: %v", err)
	}

	s.logger.Debug("get orderer config")
	orderers, consType, err := util.GetOrdererConfig(config)
	if err != nil {
		s.logger.Error("get orderer config failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "get orderer config: %v", err)
	}
	s.logger.Debug("got orderer config", zap.Int("orderers", len(orderers)), zap.String("consensus", consType.String()))

	for _, o := range orderers {
		if o.Host == req.Orderer.Host && o.Port == req.Orderer.Port {
			// orderer found, nothing to do
			return &emptypb.Empty{}, nil
		}
	}

	orderers = append(orderers, req.Orderer)
	if err = s.proceedOrderingConsenterUpdate(ctx, req.ChannelName, config, orderers); err != nil {
		return nil, status.Errorf(codes.Internal, "proceed update: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *srv) proceedOrderingConsenterUpdate(ctx context.Context, channelName string, conf *common.Config, orderers []*pb.Orderer) error {
	ordererGroup, ok := conf.ChannelGroup.Groups[channelconfig.OrdererGroupKey]
	if !ok {
		return fmt.Errorf("orderer group not found")
	}

	cType := new(orderer.ConsensusType)
	if err := proto.Unmarshal(ordererGroup.Values[channelconfig.ConsensusTypeKey].Value, cType); err != nil {
		return fmt.Errorf("unmarshal consensus value: %w", err)
	}

	var (
		mdBytes []byte
		err     error
	)

	switch util.ConvertConsensusType(cType) {
	case pb.ConsensusType_CONSENSUS_TYPE_RAFT:
		if mdBytes, err = s.createRaftOrderersUpdEnvelope(cType.Metadata, orderers); err != nil {
			return fmt.Errorf("create raft update envelope: %w", err)
		}
	case pb.ConsensusType_CONSENSUS_TYPE_BFT:
		if mdBytes, err = s.createBrfOrderersUpdEnvelope(cType.Metadata, orderers); err != nil {
			return fmt.Errorf("create bft update envelope: %w", err)
		}
	}

	newConf, ok := proto.Clone(conf).(*common.Config)
	if !ok {
		return fmt.Errorf("proto clone failed")
	}

	cType.Metadata = mdBytes
	if newConf.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey].Value, err = proto.Marshal(cType); err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	newConf.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey].Version++

	upd, err := util.Compute(conf, newConf)
	if err != nil {
		return fmt.Errorf("compute update: %w", err)
	}

	env, err := s.createUpdateEnvelope(channelName, upd)
	if err != nil {
		return fmt.Errorf("create envelope: %w", err)
	}

	return s.proceedChannelUpdate(ctx, channelName, conf, env)
}

func (s *srv) createRaftOrderersUpdEnvelope(md []byte, orderers []*pb.Orderer) ([]byte, error) {
	metadata := new(etcdraft.ConfigMetadata)
	if err := proto.Unmarshal(md, metadata); err != nil {
		return nil, fmt.Errorf("unmarshal bft metadata: %w", err)
	}

	cons := make([]*etcdraft.Consenter, 0, len(orderers))
	for _, ord := range orderers {
		cons = append(cons, &etcdraft.Consenter{
			Host:          ord.Host,
			Port:          ord.Port,
			ClientTlsCert: ord.Cert,
			ServerTlsCert: ord.Cert,
		})
	}

	metadata.Consenters = cons
	mdBytes, err := proto.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	return mdBytes, nil
}

func (s *srv) createBrfOrderersUpdEnvelope(md []byte, orderers []*pb.Orderer) ([]byte, error) {
	metadata := new(smartbft.ConfigMetadata)
	if err := proto.Unmarshal(md, metadata); err != nil {
		return nil, fmt.Errorf("unmarshal bft metadata: %w", err)
	}

	var (
		consID    uint64
		consIDMap = make(map[string]uint64)
	)
	// populate max value and consenterId map for future usage
	for _, ord := range metadata.Consenters {
		if ord.ConsenterId > consID {
			consID = ord.ConsenterId
			consIDMap[fmt.Sprintf("%s:%d", ord.Host, ord.Port)] = ord.ConsenterId
		}
	}
	// populate updated consenters value
	cons := make([]*smartbft.Consenter, 0, len(orderers))
	for _, ord := range orderers {
		cID, ok := consIDMap[fmt.Sprintf("%s:%d", ord.Host, ord.Port)]
		if !ok {
			consID++
			cID = consID
		}
		cons = append(cons, &smartbft.Consenter{
			Host:          ord.Host,
			Port:          ord.Port,
			ClientTlsCert: ord.Cert,
			ServerTlsCert: ord.Cert,
			Identity:      ord.Identity,
			ConsenterId:   cID,
		})
	}

	metadata.Consenters = cons
	mdBytes, err := proto.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	return mdBytes, nil
}
