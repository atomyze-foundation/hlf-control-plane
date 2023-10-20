package util

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestGetOrdererConfig(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("caller failed")
	}

	blockFile, err := os.ReadFile(path.Join(filepath.Dir(filename), "testdata", "ops_config.block"))
	assert.NoError(t, err)

	block := new(common.Block)
	err = proto.Unmarshal(blockFile, block)
	assert.NoError(t, err)

	envelopeConfig, err := protoutil.ExtractEnvelope(block, 0)
	assert.NoError(t, err)

	payload, err := protoutil.UnmarshalPayload(envelopeConfig.Payload)
	assert.NoError(t, err)

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	assert.NoError(t, err)

	nodes, cons, err := GetOrdererConfig(configEnvelope.Config)
	assert.NoError(t, err)
	assert.NotNil(t, nodes)
	assert.Equal(t, pb.ConsensusType_CONSENSUS_TYPE_BFT, cons)
}

func TestGetOrdererMspMap(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("caller failed")
	}

	blockFile, err := os.ReadFile(path.Join(filepath.Dir(filename), "testdata", "ops_config.block"))
	assert.NoError(t, err)

	block := new(common.Block)
	err = proto.Unmarshal(blockFile, block)
	assert.NoError(t, err)

	envelopeConfig, err := protoutil.ExtractEnvelope(block, 0)
	assert.NoError(t, err)

	payload, err := protoutil.UnmarshalPayload(envelopeConfig.Payload)
	assert.NoError(t, err)

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	assert.NoError(t, err)

	ordererMap, err := getOrgMspMap(configEnvelope.Config)
	assert.NoError(t, err)
	assert.NotNil(t, ordererMap)
}
