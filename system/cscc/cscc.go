package cscc

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
)

const chaincodeName = "cscc"

//go:generate mockery --name Client --structname CsccClient --filename client.go
type Client interface {
	GetChannels(ctx context.Context) ([]string, error)
	GetChannelConfig(ctx context.Context, channelName string) (*common.Config, error)
	JoinChain(ctx context.Context, block *common.Block) error
}
