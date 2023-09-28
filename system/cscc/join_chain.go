package cscc

import (
	"context"
	"fmt"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/util"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/protoutil"
)

func (c *cli) JoinChain(ctx context.Context, block *common.Block) error {
	// create chaincode endorsement proposal
	prop, err := c.createJoinChainProposal(block)
	if err != nil {
		return fmt.Errorf("create proposal: %w", err)
	}

	// create signed proposal using identity
	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return fmt.Errorf("create singed proposal: %w", err)
	}

	// process signed proposal on peer
	if err = c.processJoinChainProposal(ctx, signedProp); err != nil {
		return fmt.Errorf("process proposal: %w", err)
	}

	return nil
}

func (c *cli) createJoinChainProposal(block *common.Block) (*pb.Proposal, error) {
	blockBytes, err := proto.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("marshal block: %w", err)
	}

	// Build the spec
	input := &pb.ChaincodeInput{Args: [][]byte{[]byte(cscc.JoinChain), blockBytes}}
	invocation := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
			ChaincodeId: &pb.ChaincodeID{Name: "cscc"},
			Input:       input,
		},
	}

	cr, err := c.id.Serialize()
	if err != nil {
		return nil, fmt.Errorf("signer serialize: %w", err)
	}
	prop, _, err := protoutil.CreateProposalFromCIS(common.HeaderType_ENDORSER_TRANSACTION, "", invocation, cr)
	if err != nil {
		return nil, fmt.Errorf("cannot create proposal, due to %w", err)
	}
	return prop, nil
}

func (c *cli) processJoinChainProposal(ctx context.Context, prop *pb.SignedProposal) error {
	resp, err := c.cli.ProcessProposal(ctx, prop)
	if err != nil {
		return fmt.Errorf("endorse proposal: %w", err)
	}

	if resp == nil {
		return fmt.Errorf("empty response")
	}

	if resp.Response.Status != 0 && resp.Response.Status != 200 {
		return fmt.Errorf("bad proposal response %d: %s", resp.Response.Status, resp.Response.Message)
	}

	return nil
}
