package lifecycle

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
)

type InstalledChaincode struct {
	PackageID string
	Label     string
}

type ApproveRequest struct {
	Name              string
	PackageID         string
	InitRequired      bool
	SignaturePolicy   []byte
	Sequence          int64
	Version           string
	EndorsementPlugin string
	ValidationPlugin  string
}

type ApproveResponse struct {
	TxID      string
	Responses []*pb.ProposalResponse
	Proposal  *pb.Proposal
}

const CcName = "_lifecycle"

type Client interface {
	QueryInstalled(ctx context.Context) ([]InstalledChaincode, error)
	QueryApproved(ctx context.Context, channelName, chaincodeName string) (*lb.QueryApprovedChaincodeDefinitionResult, error)
	QueryCommitted(ctx context.Context, channelName string) ([]*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition, error)

	ApproveForMyOrg(ctx context.Context, channelName string, req *ApproveRequest, endCli ...pb.EndorserClient) (string, *common.Envelope, error)
	CheckCommitReadiness(ctx context.Context, channelName string, args *lb.CheckCommitReadinessArgs) (*lb.CheckCommitReadinessResult, error)
	Commit(ctx context.Context, channelName string, args *lb.CommitChaincodeDefinitionArgs, endCli ...pb.EndorserClient) (string, *common.Envelope, error)

	Install(ctx context.Context, pkg []byte) error
}
