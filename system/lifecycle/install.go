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

func (c *cli) Install(ctx context.Context, pkgBytes []byte) error {
	serializedSigner, err := c.id.Serialize()
	if err != nil {
		return fmt.Errorf("serialize signer: %w", err)
	}

	prop, err := c.createInstallProposal(pkgBytes, serializedSigner)
	if err != nil {
		return fmt.Errorf("create proposal: %w", err)
	}

	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return fmt.Errorf("sign proposal: %w", err)
	}

	return c.processInstallProposal(ctx, signedProp)
}

func (c *cli) createInstallProposal(pkgBytes []byte, creatorBytes []byte) (*pb.Proposal, error) {
	installChaincodeArgs := &lb.InstallChaincodeArgs{
		ChaincodeInstallPackage: pkgBytes,
	}

	installChaincodeArgsBytes, err := proto.Marshal(installChaincodeArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal InstallChaincodeArgs")
	}

	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("InstallChaincode"), installChaincodeArgsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: CcName},
			Input:       ccInput,
		},
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, "", cis, creatorBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create proposal for ChaincodeInvocationSpec")
	}

	return proposal, nil
}

func (c *cli) processInstallProposal(ctx context.Context, signedProposal *pb.SignedProposal) error {
	proposalResponse, err := c.cli.ProcessProposal(ctx, signedProposal)
	if err != nil {
		return fmt.Errorf("process proposal: %w", err)
	}

	if proposalResponse == nil {
		return errors.New("chaincode install failed: received nil proposal response")
	}

	if proposalResponse.Response == nil {
		return errors.New("chaincode install failed: received proposal response with nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("chaincode install failed with status: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}

	icr := &lb.InstallChaincodeResult{}
	err = proto.Unmarshal(proposalResponse.Response.Payload, icr)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal proposal response's response payload")
	}

	return nil
}
