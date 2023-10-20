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

const commitFunc = "CommitChaincodeDefinition"

func (c *cli) Commit(ctx context.Context, channelName string, args *lb.CommitChaincodeDefinitionArgs, endCli ...pb.EndorserClient) (string, *cb.Envelope, error) {
	prop, txID, err := c.createCommitProposal(channelName, args)
	if err != nil {
		return "", nil, fmt.Errorf("create proposal: %w", err)
	}

	signedProp, err := util.SignProposal(prop, c.id)
	if err != nil {
		return "", nil, fmt.Errorf("sign proposal: %w", err)
	}

	responses, err := util.EndorsePeers(ctx, signedProp, endCli...)
	if err != nil {
		return "", nil, fmt.Errorf("endorse peers: %w", err)
	}

	env, err := protoutil.CreateSignedTx(prop, c.id, responses...)
	if err != nil {
		return "", nil, fmt.Errorf("create signed tx: %w", err)
	}

	return txID, env, nil
}

func (c *cli) createCommitProposal(channelName string, args *lb.CommitChaincodeDefinitionArgs) (*pb.Proposal, string, error) {
	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, "", err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte(commitFunc), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: CcName},
			Input:       ccInput,
		},
	}

	creator, err := c.id.Serialize()
	if err != nil {
		return nil, "", fmt.Errorf("serialize creator: %w", err)
	}

	nonce, err := util.GetRandomNonce()
	if err != nil {
		return nil, "", fmt.Errorf("get nonce: %w", err)
	}

	txID := protoutil.ComputeTxID(nonce, creator)
	return protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(txID, cb.HeaderType_ENDORSER_TRANSACTION, channelName, cis, nonce, creator, nil)
}
