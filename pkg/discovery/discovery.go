package discovery

import (
	"context"

	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/hyperledger/fabric-protos-go/discovery"
)

type Client interface {
	GetPeers(ctx context.Context, channelName string) ([]*proto.DiscoveryPeer, error)
	GetEndorsers(ctx context.Context, channelName, ccName string) ([]*proto.DiscoveryPeer, error)
	GetConfig(ctx context.Context, channelName string) (*discovery.ConfigResult, error)
}
