package util

import (
	"fmt"
	"sort"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/peer"
	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric-protos-go/orderer/smartbft"
	pp "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"golang.org/x/exp/slices"
)

func GetOrdererConfig(conf *common.Config) ([]*pb.Orderer, pb.ConsensusType, error) { //nolint:funlen
	ordererGroup, ok := conf.ChannelGroup.Groups[channelconfig.OrdererGroupKey]
	if !ok {
		return nil, 0, fmt.Errorf("orderer group not found")
	}

	var err error

	cType := new(orderer.ConsensusType)
	if err = proto.Unmarshal(ordererGroup.Values[channelconfig.ConsensusTypeKey].Value, cType); err != nil {
		return nil, 0, fmt.Errorf("unmarshal consensus value: %w", err)
	}

	consensusType := ConvertConsensusType(cType)
	nodes := make([]*pb.Orderer, 0)

	switch consensusType {
	case pb.ConsensusType_CONSENSUS_TYPE_BFT:
		metadata := new(smartbft.ConfigMetadata)
		if err = proto.Unmarshal(cType.Metadata, metadata); err != nil {
			return nil, consensusType, fmt.Errorf("unmarshal bft metadata: %w", err)
		}
		orgMap, err := getOrgMspMap(conf)
		if err != nil {
			return nil, consensusType, fmt.Errorf("get orderer msp: %w", err)
		}
		for _, c := range metadata.Consenters {
			mspConf, ok := orgMap[c.MspId]
			if !ok {
				return nil, consensusType, fmt.Errorf("orderer not found")
			}
			nodes = append(nodes, &pb.Orderer{
				Host:        c.Host,
				Port:        c.Port,
				Cert:        c.ServerTlsCert,
				CaCerts:     append(mspConf.RootCerts, mspConf.TlsRootCerts...),
				MspId:       c.MspId,
				Identity:    c.Identity,
				ConsenterId: c.ConsenterId,
			})
		}
	case pb.ConsensusType_CONSENSUS_TYPE_RAFT:
		metadata := new(etcdraft.ConfigMetadata)
		if err = proto.Unmarshal(cType.Metadata, metadata); err != nil {
			return nil, consensusType, fmt.Errorf("unamrshal bft metadata: %w", err)
		}
		consMap, err := getConsenterMap(ordererGroup)
		if err != nil {
			return nil, consensusType, fmt.Errorf("get consenter map: %w", err)
		}
		for _, c := range metadata.Consenters {
			if v, ok := consMap[fmt.Sprintf("%s:%d", c.Host, c.Port)]; ok {
				nodes = append(nodes, &pb.Orderer{
					Host:    c.Host,
					Port:    c.Port,
					Cert:    c.ServerTlsCert,
					MspId:   v.MspId,
					CaCerts: v.CaCerts,
				})
				continue
			}
			nodes = append(nodes, &pb.Orderer{
				Host:  c.Host,
				Port:  c.Port,
				Cert:  c.ServerTlsCert,
				MspId: "",
			})
		}
	}

	return nodes, consensusType, err
}

func getConsenterMap(ordererGroup *common.ConfigGroup) (map[string]*pb.Orderer, error) {
	ret := make(map[string]*pb.Orderer)
	for _, orgGroup := range ordererGroup.Groups {
		mspConf, err := GetMspConfig(orgGroup)
		if err != nil {
			return nil, fmt.Errorf("get msp config: %w", err)
		}
		endpoints, err := GetEndpoints(orgGroup)
		if err != nil {
			return nil, fmt.Errorf("get endpoints config: %w", err)
		}
		if endpoints == nil {
			continue
		}

		for _, host := range endpoints.Addresses {
			ret[host] = &pb.Orderer{
				MspId:   mspConf.Name,
				CaCerts: append(mspConf.RootCerts, mspConf.TlsRootCerts...),
			}
		}
	}
	return ret, nil
}

func ConvertConsensusType(cType *orderer.ConsensusType) pb.ConsensusType {
	switch cType.Type {
	case "smartbft":
		return pb.ConsensusType_CONSENSUS_TYPE_BFT
	case "etcdraft":
		return pb.ConsensusType_CONSENSUS_TYPE_RAFT
	}
	return pb.ConsensusType_CONSENSUS_TYPE_UNSPECIFIED
}

func getOrgMspMap(conf *common.Config) (map[string]*msp.FabricMSPConfig, error) {
	ordererGroup, ok := conf.ChannelGroup.Groups[channelconfig.OrdererGroupKey]
	if !ok {
		return nil, fmt.Errorf("orderer group not found")
	}

	orgMap := make(map[string]*msp.FabricMSPConfig)
	for _, org := range ordererGroup.Groups {
		mspConf, err := GetMspConfig(org)
		if err != nil {
			return nil, fmt.Errorf("get msp config: %w", err)
		}
		orgMap[mspConf.Name] = mspConf
	}
	return orgMap, nil
}

func GetPeerConfig(conf *common.Config, mspIds ...string) ([]*peer.Peer, error) {
	// sort msp identifiers for future search usage
	sort.Strings(mspIds)
	applicationGroup, ok := conf.ChannelGroup.Groups[channelconfig.ApplicationGroupKey]
	if !ok {
		return nil, fmt.Errorf("application group not found")
	}
	peers := make([]*peer.Peer, 0)
	for _, org := range applicationGroup.Groups {
		mspConf, err := GetMspConfig(org)
		if err != nil {
			return nil, fmt.Errorf("get msp config: %w", err)
		}
		if len(mspIds) == 0 || slices.Contains(mspIds, mspConf.Name) {
			anchorPeersValue, ok := org.Values[channelconfig.AnchorPeersKey]
			if !ok {
				return nil, fmt.Errorf("anchor peer key not found")
			}
			var ap pp.AnchorPeers
			if err = proto.Unmarshal(anchorPeersValue.Value, &ap); err != nil {
				return nil, fmt.Errorf("unmarshal anchor peers: %w", err)
			}
			for _, p := range ap.AnchorPeers {
				peers = append(peers, &peer.Peer{
					Host:           p.Host,
					Port:           p.Port,
					MspID:          mspConf.Name,
					CACertificates: append(mspConf.RootCerts, mspConf.TlsRootCerts...),
				})
			}
		}
	}
	return peers, nil
}

func GetAnchorPeerConfig(conf *common.Config, mspID string) ([]*pp.AnchorPeer, error) {
	applicationGroup, ok := conf.ChannelGroup.Groups[channelconfig.ApplicationGroupKey]
	if !ok {
		return nil, fmt.Errorf("application group not found")
	}
	for _, org := range applicationGroup.Groups {
		mspConf, err := GetMspConfig(org)
		if err != nil {
			return nil, fmt.Errorf("get msp config: %w", err)
		}
		if mspConf.Name == mspID {
			anchorPeersValue, ok := org.Values[channelconfig.AnchorPeersKey]
			if !ok {
				return nil, nil
			}
			var ap pp.AnchorPeers
			if err := proto.Unmarshal(anchorPeersValue.Value, &ap); err != nil {
				return nil, fmt.Errorf("unmarshal anchor peers: %w", err)
			}
			return ap.AnchorPeers, nil
		}
	}
	return nil, fmt.Errorf("anchor peers not found for MSP: %s", mspID)
}

// GetMspConfig returns fabric msp config from presented config group
func GetMspConfig(group *common.ConfigGroup) (*msp.FabricMSPConfig, error) {
	mspValue, ok := group.Values[channelconfig.MSPKey]
	if !ok {
		return nil, fmt.Errorf("msp key not found")
	}
	var mspConf msp.MSPConfig
	if err := proto.Unmarshal(mspValue.Value, &mspConf); err != nil {
		return nil, fmt.Errorf("unmarshal msp: %w", err)
	}
	var conf msp.FabricMSPConfig
	if err := proto.Unmarshal(mspConf.Config, &conf); err != nil {
		return nil, fmt.Errorf("unmarshal fabric msp config: %w", err)
	}

	return &conf, nil
}

func GetEndpoints(group *common.ConfigGroup) (*common.OrdererAddresses, error) {
	addrValue, ok := group.Values[channelconfig.EndpointsKey]
	if !ok {
		return nil, nil //nolint:nilnil
	}
	var addr common.OrdererAddresses
	if err := proto.Unmarshal(addrValue.Value, &addr); err != nil {
		return nil, fmt.Errorf("unmarshal orderer addresses: %w", err)
	}
	return &addr, nil
}
