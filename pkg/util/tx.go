package util

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
)

func GetTxIDFromEnvelope(env *common.Envelope) (string, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return "", fmt.Errorf("unmarshal payload: %w", err)
	}

	chHeader, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return "", fmt.Errorf("unmarshal channel header: %w", err)
	}

	return chHeader.TxId, nil
}
