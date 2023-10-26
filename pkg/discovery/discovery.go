package discovery

import (
	"context"

	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/hyperledger/fabric-protos-go/discovery"
)

// Client is an interface that defines methods for interacting with a discovery client to obtain information
// about peers, endorsers, and configuration related to a specific channel.
type Client interface {
	// GetPeers retrieves information about peers that belong to a specified channel.
	// It returns a list of discovery peers and any encountered error.
	GetPeers(ctx context.Context, channelName string) ([]*proto.DiscoveryPeer, error)

	// GetEndorsers retrieves information about endorsers for a specific chaincode on a channel.
	// It returns a list of discovery peers acting as endorsers and any encountered error.
	GetEndorsers(ctx context.Context, channelName, ccName string) ([]*proto.DiscoveryPeer, error)

	// GetConfig retrieves configuration information for a specified channel.
	// It returns the configuration result and any encountered error.
	GetConfig(ctx context.Context, channelName string) (*discovery.ConfigResult, error)
}
