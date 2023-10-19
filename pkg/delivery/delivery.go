package delivery

import (
	"context"

	pb "github.com/hyperledger/fabric-protos-go/peer"
)

type Client interface {
	SubscribeTx(ctx context.Context, channelName string, txID string) (pb.TxValidationCode, error)
	SubscribeTxAll(ctx context.Context, channelName string, txID string) error
}
