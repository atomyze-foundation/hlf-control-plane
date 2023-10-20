package discovery

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/peer"
	"github.com/atomyze-foundation/hlf-control-plane/proto"
	fd "github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	discovery "github.com/hyperledger/fabric/discovery/client"
	"github.com/hyperledger/fabric/protoutil"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type cli struct {
	log   *zap.Logger
	mspID string
	pool  peer.Pool
	id    protoutil.Signer
	dCli  *discovery.Client
}

func (c *cli) GetPeers(ctx context.Context, channelName string) ([]*proto.DiscoveryPeer, error) {
	req := discovery.NewRequest()
	req.OfChannel(channelName).AddPeersQuery()

	resp, err := c.processResponse(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("process response: %w", err)
	}
	peers, err := resp.ForChannel(channelName).Peers()
	if err != nil {
		return nil, fmt.Errorf("get peers from response: %w", err)
	}

	return c.getPeerResponse(peers)
}

func (c *cli) GetEndorsers(ctx context.Context, channelName, ccName string) ([]*proto.DiscoveryPeer, error) {
	ccCall := &pb.ChaincodeCall{Name: ccName}
	req, err := discovery.NewRequest().OfChannel(channelName).AddEndorsersQuery(&pb.ChaincodeInterest{Chaincodes: []*pb.ChaincodeCall{ccCall}})
	if err != nil {
		return nil, fmt.Errorf("create discovery query: %w", err)
	}

	resp, err := c.processResponse(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("process response: %w", err)
	}

	endorsers, err := resp.ForChannel(channelName).Endorsers([]*pb.ChaincodeCall{ccCall}, discovery.NewFilter(discovery.PrioritiesByHeight, discovery.NoExclusion))
	if err != nil {
		return nil, fmt.Errorf("get endorsers from response: %w", err)
	}

	return c.getPeerResponse(endorsers)
}

func (c *cli) GetConfig(ctx context.Context, channelName string) (*fd.ConfigResult, error) {
	req := discovery.NewRequest().OfChannel(channelName).AddConfigQuery()
	resp, err := c.processResponse(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("process response: %w", err)
	}

	return resp.ForChannel(channelName).Config()
}

func (c *cli) processResponse(ctx context.Context, req *discovery.Request) (discovery.Response, error) {
	sID, err := c.id.Serialize()
	if err != nil {
		return nil, fmt.Errorf("serialize identity: %w", err)
	}

	resp, err := c.dCli.Send(ctx, req, &fd.AuthInfo{
		ClientIdentity:    sID,
		ClientTlsCertHash: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	return resp, nil
}

func (c *cli) getPeerResponse(peers []*discovery.Peer) ([]*proto.DiscoveryPeer, error) {
	discoveryPeers := make([]*proto.DiscoveryPeer, 0, len(peers))
	for _, p := range peers {
		id, err := protoutil.UnmarshalSerializedIdentity(p.Identity)
		if err != nil {
			return nil, fmt.Errorf("deserialize identity: %w", err)
		}

		b, _ := pem.Decode(id.IdBytes)
		if b == nil {
			return nil, fmt.Errorf("identity is empty")
		}

		cert, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}

		key, ok := cert.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("parse public key: %w", err)
		}

		raw := elliptic.Marshal(key.Curve, key.X, key.Y)
		hash := sha256.Sum256(raw)

		ccList := make([]string, 0, len(p.StateInfoMessage.GetStateInfo().Properties.Chaincodes))
		for _, cc := range p.StateInfoMessage.GetStateInfo().Properties.Chaincodes {
			ccList = append(ccList, cc.Name)
		}

		parts := strings.Split(p.AliveMessage.GetAliveMsg().Membership.Endpoint, ":")
		if len(parts) > 2 { //nolint:gomnd
			return nil, fmt.Errorf("invalid endpoint")
		}

		port, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid parse port: %w", err)
		}

		discoveryPeers = append(discoveryPeers, &proto.DiscoveryPeer{
			MspId:       p.MSPID,
			BlockNumber: p.StateInfoMessage.GossipMessage.GetStateInfo().GetProperties().LedgerHeight,
			Host:        parts[0],
			Port:        int32(port),
			Cert: &proto.DiscoveryPeer_Certificate{
				Raw:        string(id.IdBytes),
				Ski:        fmt.Sprintf("%x", hash[:]),
				Domains:    cert.DNSNames,
				DateExpire: timestamppb.New(cert.NotAfter),
			},
			Chaincodes: ccList,
		})
	}
	return discoveryPeers, nil
}

func NewClient(log *zap.Logger, mspID string, pool peer.Pool, id protoutil.Signer) Client {
	l := log.Named("discovery")

	c := &cli{
		log:   l,
		mspID: mspID,
		pool:  pool,
		id:    id,
	}
	c.dCli = discovery.NewClient(func() (*grpc.ClientConn, error) {
		conn, _, err := pool.GetConnection(mspID)
		return conn, err
	}, func(msg []byte) ([]byte, error) {
		return id.Sign(msg)
	}, 0)
	return c
}
