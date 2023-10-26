package qscc

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
)

// Client is an interface that defines methods for interacting with a blockchain to obtain information
// about a specific chain's status and information.
type Client interface {
	// GetChainInfo retrieves information about a specific blockchain chain, such as its status, height, and more.
	//
	// Parameters:
	// - ctx: The context for the operation.
	// - channelName: The name of the channel associated with the blockchain chain.
	//
	// Returns:
	// - *common.BlockchainInfo: Information about the blockchain chain.
	// - error: An error, if any, encountered during the retrieval process.
	GetChainInfo(ctx context.Context, channelName string) (*common.BlockchainInfo, error)
}
