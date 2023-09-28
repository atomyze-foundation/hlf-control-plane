package lifecycle

import (
	"context"
	"fmt"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const queryCommittedFunc = "QueryChaincodeDefinitions"

func (c *cli) QueryCommitted(ctx context.Context, channelName string) ([]*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition, error) {
	prop, err := c.createQueryCommittedProposal(channelName)
	if err != nil {
		return nil, fmt.Errorf("create proposal: %w", err)
	}

	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return nil, fmt.Errorf("sign proposal: %w", err)
	}

	return c.processQueryCommittedProposal(ctx, signedProp)
}

func (c *cli) createQueryCommittedProposal(channelName string) (*pb.Proposal, error) {
	argsBytes, err := proto.Marshal(&lb.QueryChaincodeDefinitionsArgs{})
	if err != nil {
		return nil, fmt.Errorf("create proposal: %w", err)
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte(queryCommittedFunc), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: CcName},
			Input:       ccInput,
		},
	}

	signerSerialized, err := c.id.Serialize()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to serialize identity")
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, channelName, cis, signerSerialized)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create ChaincodeInvocationSpec proposal")
	}

	return proposal, nil
}

func (c *cli) processQueryCommittedProposal(ctx context.Context, singedProp *pb.SignedProposal) ([]*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition, error) {
	resp, err := c.cli.ProcessProposal(ctx, singedProp)
	if err != nil {
		return nil, fmt.Errorf("endorse proposal: %w", err)
	}

	if resp == nil {
		return nil, fmt.Errorf("received nil proposal response")
	}

	if resp.Response == nil {
		return nil, fmt.Errorf("received proposal response with nil response")
	}

	if resp.Response.Status != int32(cb.Status_SUCCESS) {
		return nil, fmt.Errorf("query failed with status: %d - %s", resp.Response.Status, resp.Response.Message)
	}

	var result lb.QueryChaincodeDefinitionsResult
	if err = proto.Unmarshal(resp.Response.Payload, &result); err != nil {
		return nil, fmt.Errorf("proto unmarshal: %w", err)
	}

	return result.ChaincodeDefinitions, nil
}
