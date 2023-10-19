package mocks

import (
	"context"
	"errors"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"google.golang.org/grpc"
)

type ordererClient struct {
	broadCast bool
	deliver   bool
}

func (o ordererClient) Broadcast(_ context.Context, _ ...grpc.CallOption) (orderer.AtomicBroadcast_BroadcastClient, error) {
	if o.broadCast {
		return &ordererBroadCastClient{}, nil
	}
	return nil, errors.New("broadcast err")
}

func (o ordererClient) Deliver(_ context.Context, _ ...grpc.CallOption) (orderer.AtomicBroadcast_DeliverClient, error) {
	if o.deliver {
		return nil, nil
	}
	return nil, errors.New("deliver err")
}

func NewOrdererClient(broadCast, deliver bool) orderer.AtomicBroadcastClient {
	return &ordererClient{broadCast: broadCast, deliver: deliver}
}

type ordererBroadCastClient struct {
	grpc.ClientStream
}

func (o *ordererBroadCastClient) Send(_ *common.Envelope) error {
	return nil
}

func (o *ordererBroadCastClient) Recv() (*orderer.BroadcastResponse, error) {
	return &orderer.BroadcastResponse{
		Status: common.Status_SUCCESS,
		Info:   "success",
	}, nil
}
