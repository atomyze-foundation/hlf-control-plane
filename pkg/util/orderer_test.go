package util

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/atomyze-foundation/hlf-control-plane/test/mocks"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestOrdererBroadcast(t *testing.T) {
	corrOrderer := mocks.NewOrdererClient(true, true)
	failedOrderer := mocks.NewOrdererClient(false, false)

	log := zap.NewNop()

	type args struct {
		ctx            context.Context
		l              *zap.Logger
		env            *common.Envelope
		quorum         int
		ordererClients []orderer.AtomicBroadcastClient
	}

	quorum := func(n int) (q int) {
		f := (n - 1) / 3
		q = int(math.Ceil((float64(n) + float64(f) + 1) / 2.0))
		return
	}

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Check all correct orderers",
			args: args{
				ctx:            context.Background(),
				l:              log,
				env:            nil,
				quorum:         quorum(3),
				ordererClients: []orderer.AtomicBroadcastClient{corrOrderer, corrOrderer, corrOrderer},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return false
			},
		},
		{
			name: "Check 50/50 orderers",
			args: args{
				ctx:            context.Background(),
				l:              log,
				env:            nil,
				quorum:         quorum(7),
				ordererClients: []orderer.AtomicBroadcastClient{corrOrderer, corrOrderer, corrOrderer, corrOrderer, failedOrderer, failedOrderer, failedOrderer},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return false
			},
		},
		{
			name: "Check all failed orderers",
			args: args{
				ctx:            context.Background(),
				l:              log,
				env:            nil,
				quorum:         quorum(3),
				ordererClients: []orderer.AtomicBroadcastClient{failedOrderer, failedOrderer, failedOrderer},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, OrdererBroadcast(tt.args.ctx, tt.args.l, tt.args.env, tt.args.quorum, tt.args.ordererClients...), fmt.Sprintf("OrdererBroadcast(%v, %v, %v, %v)", tt.args.ctx, tt.args.l, tt.args.env, tt.args.quorum))
		})
	}
}
