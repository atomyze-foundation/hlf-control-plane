package qscc

import (
	"context"
	"fmt"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/scc/qscc"
	"github.com/hyperledger/fabric/protoutil"
)

func (c *cli) GetChainInfo(ctx context.Context, channelName string) (*common.BlockchainInfo, error) {
	// create chaincode endorsement proposal
	prop, err := c.getGetChainInfoProposal(channelName)
	if err != nil {
		return nil, fmt.Errorf("get proposal: %w", err)
	}

	// create signed proposal using identity
	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return nil, fmt.Errorf("create singed proposal: %w", err)
	}

	// process signed proposal on peer
	chainInfo, err := c.processGetChainInfoProposal(ctx, signedProp)
	if err != nil {
		return nil, fmt.Errorf("process proposal: %w", err)
	}
	return chainInfo, nil
}

func (c *cli) getGetChainInfoProposal(channelName string) (*peer.Proposal, error) {
	invocation := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_Type(peer.ChaincodeSpec_Type_value["GOLANG"]),
			ChaincodeId: &peer.ChaincodeID{Name: "qscc"},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte(qscc.GetChainInfo), []byte(channelName)}},
		},
	}

	cr, err := c.id.Serialize()
	if err != nil {
		return nil, fmt.Errorf("signer serialize: %w", err)
	}

	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "", invocation, cr)
	if err != nil {
		return nil, fmt.Errorf("create proposal: %w", err)
	}
	return prop, nil
}

func (c *cli) processGetChainInfoProposal(ctx context.Context, prop *peer.SignedProposal) (*common.BlockchainInfo, error) {
	resp, err := c.cli.ProcessProposal(ctx, prop)
	if err != nil {
		return nil, fmt.Errorf("process proposal: %w", err)
	}

	if resp.Response == nil || resp.Response.Status != 200 {
		return nil, fmt.Errorf("received bad response, status %d: %s", resp.Response.Status, resp.Response.Message)
	}

	blockChainInfo := &common.BlockchainInfo{}
	if err = proto.Unmarshal(resp.Response.Payload, blockChainInfo); err != nil {
		return nil, fmt.Errorf("proto unmarshal: %w", err)
	}

	return blockChainInfo, nil
}
