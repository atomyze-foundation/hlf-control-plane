package qscc

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
)

type Client interface {
	GetChainInfo(ctx context.Context, channelName string) (*common.BlockchainInfo, error)
}
