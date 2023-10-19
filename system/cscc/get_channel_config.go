package cscc

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
)

func (c *cli) GetChannelConfig(ctx context.Context, channelName string) (*common.Config, error) {
	// create chaincode endorsement proposal
	prop, err := c.getGetChannelConfigProposal(channelName)
	if err != nil {
		return nil, fmt.Errorf("get proposal: %w", err)
	}

	// create signed proposal using identity
	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return nil, fmt.Errorf("create singed proposal: %w", err)
	}

	// process signed proposal on peer
	config, err := c.processGetChannelConfigProposal(ctx, signedProp)
	if err != nil {
		return nil, fmt.Errorf("process proposal: %w", err)
	}
	return config, nil
}

func (c *cli) getGetChannelConfigProposal(channelName string) (*peer.Proposal, error) {
	// query cscc for chain config block
	invocation := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_Type(peer.ChaincodeSpec_Type_value["GOLANG"]),
			ChaincodeId: &peer.ChaincodeID{Name: "cscc"},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte(cscc.GetChannelConfig), []byte(channelName)}},
		},
	}
	creator, err := c.id.Serialize()
	if err != nil {
		return nil, fmt.Errorf("signer serialize: %w", err)
	}

	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_CONFIG, "", invocation, creator)
	if err != nil {
		return nil, fmt.Errorf("cannot create proposal, due to %w", err)
	}
	return prop, nil
}

func (c *cli) processGetChannelConfigProposal(ctx context.Context, prop *peer.SignedProposal) (*common.Config, error) {
	resp, err := c.cli.ProcessProposal(ctx, prop)
	if err != nil {
		return nil, fmt.Errorf("process proposal: %w", err)
	}

	if resp.Response == nil || resp.Response.Status != 200 {
		return nil, fmt.Errorf("received bad response, status %d: %s", resp.Response.Status, resp.Response.Message)
	}

	blockChainInfo := &common.Config{}
	if err = proto.Unmarshal(resp.Response.Payload, blockChainInfo); err != nil {
		return nil, fmt.Errorf("proto unmarshal: %w", err)
	}

	return blockChainInfo, nil
}
