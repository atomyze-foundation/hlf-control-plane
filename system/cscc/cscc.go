package cscc

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
)

const chaincodeName = "cscc"

// Client is an interface that defines methods for interacting with a blockchain network,
// specifically related to channels.
//
//go:generate mockery --name Client --structname CsccClient --filename client.go
type Client interface {
	// GetChannels retrieves a list of channel names in the blockchain network.
	GetChannels(ctx context.Context) ([]string, error)

	// GetChannelConfig retrieves the configuration of a specific channel.
	GetChannelConfig(ctx context.Context, channelName string) (*common.Config, error)

	// JoinChain joins the client to a blockchain network using a specified block.
	JoinChain(ctx context.Context, block *common.Block) error
}
