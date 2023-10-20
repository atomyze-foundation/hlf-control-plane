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

func (c *cli) QueryInstalled(ctx context.Context) ([]InstalledChaincode, error) {
	// create proposal for peer query
	prop, err := c.createQueryInstalledProposal()
	if err != nil {
		return nil, fmt.Errorf("create proposal: %w", err)
	}

	// create signed proposal using identity
	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return nil, fmt.Errorf("create singed proposal: %w", err)
	}

	// process proposal on each peer
	res, err := c.processQueryInstalledProposal(ctx, signedProp)
	if err != nil {
		return nil, fmt.Errorf("process proposal failed: %w", err)
	}

	// populate result from chaincode response
	result := make([]InstalledChaincode, 0)
	for _, cc := range res.InstalledChaincodes {
		result = append(result, InstalledChaincode{PackageID: cc.PackageId, Label: cc.Label})
	}

	return result, nil
}

func (c *cli) processQueryInstalledProposal(ctx context.Context, prop *pb.SignedProposal) (*lb.QueryInstalledChaincodesResult, error) {
	propResp, err := c.cli.ProcessProposal(ctx, prop)
	if err != nil {
		return nil, fmt.Errorf("failed to endorse proposal: %w", err)
	}

	if propResp == nil {
		return nil, errors.New("received nil proposal response")
	}

	if propResp.Response == nil {
		return nil, errors.New("received proposal response with nil response")
	}

	if propResp.Response.Status != int32(cb.Status_SUCCESS) {
		return nil, errors.Errorf("query failed with status: %d - %s", propResp.Response.Status, propResp.Response.Message)
	}

	var res lb.QueryInstalledChaincodesResult

	if err = proto.Unmarshal(propResp.Response.Payload, &res); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	return &res, nil
}

func (c *cli) createQueryInstalledProposal() (*pb.Proposal, error) {
	args := &lb.QueryInstalledChaincodesArgs{}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal args")
	}

	ccInput := &pb.ChaincodeInput{
		Args: [][]byte{[]byte("QueryInstalledChaincodes"), argsBytes},
	}

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

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, "", cis, signerSerialized)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create ChaincodeInvocationSpec proposal")
	}

	return proposal, nil
}
