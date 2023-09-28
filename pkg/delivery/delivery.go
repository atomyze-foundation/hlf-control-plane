package delivery

import (
	"context"

	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// Client is an interface that defines methods for interacting with a transaction client.
type Client interface {
	// SubscribeTx allows a client to subscribe to a specific transaction identified by its ID on a given channel.
	// It returns the validation code of the transaction and any encountered error.
	SubscribeTx(ctx context.Context, channelName string, txID string) (pb.TxValidationCode, error)
	// SubscribeTxAll allows a client to subscribe to all transactions on a given channel.
	// It returns any encountered error during the subscription process.
	SubscribeTxAll(ctx context.Context, channelName string, txID string) error
}
