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
)

const checkCommitReadinessFunc = "CheckCommitReadiness"

func (c *cli) CheckCommitReadiness(ctx context.Context, channelName string, args *lb.CheckCommitReadinessArgs) (*lb.CheckCommitReadinessResult, error) {
	prop, err := c.createCheckCommitReadinessProposal(channelName, args)
	if err != nil {
		return nil, fmt.Errorf("create proposal: %w", err)
	}

	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return nil, fmt.Errorf("sign proposal: %w", err)
	}

	return c.processCheckCommitReadinessProposal(ctx, signedProp)
}

func (c *cli) createCheckCommitReadinessProposal(channelName string, args *lb.CheckCommitReadinessArgs) (*pb.Proposal, error) {
	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal args: %w", err)
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte(checkCommitReadinessFunc), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: CcName},
			Input:       ccInput,
		},
	}

	creatorBytes, err := c.id.Serialize()
	if err != nil {
		return nil, fmt.Errorf("serialize creator: %w", err)
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, channelName, cis, creatorBytes)
	if err != nil {
		return nil, err
	}

	return proposal, nil
}

func (c *cli) processCheckCommitReadinessProposal(ctx context.Context, signedProp *pb.SignedProposal) (*lb.CheckCommitReadinessResult, error) {
	resp, err := c.cli.ProcessProposal(ctx, signedProp)
	if err != nil {
		return nil, fmt.Errorf("process proposal: %w", err)
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

	var res lb.CheckCommitReadinessResult
	if err = proto.Unmarshal(resp.Response.Payload, &res); err != nil {
		return nil, fmt.Errorf("proto unmarshal: %w", err)
	}

	return &res, nil
}
