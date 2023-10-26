package lifecycle

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
)

// InstalledChaincode represents information about an installed chaincode package.
type InstalledChaincode struct {
	// PackageID is the unique identifier for the installed chaincode package.
	PackageID string

	// Label is a label or name associated with the installed chaincode.
	Label string
}

// ApproveRequest represents a request object containing information required
// for approving a chaincode definition.
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

// ApproveResponse represents a response object containing information about an approval action.
type ApproveResponse struct {
	TxID      string
	Responses []*pb.ProposalResponse
	Proposal  *pb.Proposal
}

// CcName represents hlf lifecycle chaincode name.
const CcName = "_lifecycle"

// Client is an interface that defines methods for interacting with a blockchain network, specifically related to chaincode.
type Client interface {
	// QueryInstalled retrieves information about installed chaincodes on the network.
	QueryInstalled(ctx context.Context) ([]InstalledChaincode, error)

	// QueryApproved retrieves information about an approved chaincode on a specific channel.
	QueryApproved(ctx context.Context, channelName, chaincodeName string) (*lb.QueryApprovedChaincodeDefinitionResult, error)

	// QueryCommitted retrieves information about committed chaincode definitions on a specific channel.
	QueryCommitted(ctx context.Context, channelName string) ([]*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition, error)

	// ApproveForMyOrg approves a chaincode definition for a specific channel on behalf of the client's organization.
	ApproveForMyOrg(ctx context.Context, channelName string, req *ApproveRequest, endCli ...pb.EndorserClient) (string, *common.Envelope, error)

	// CheckCommitReadiness checks the readiness for committing a chaincode definition to a channel.
	CheckCommitReadiness(ctx context.Context, channelName string, args *lb.CheckCommitReadinessArgs) (*lb.CheckCommitReadinessResult, error)

	// Commit commits a chaincode definition to a specific channel.
	Commit(ctx context.Context, channelName string, args *lb.CommitChaincodeDefinitionArgs, endCli ...pb.EndorserClient) (string, *common.Envelope, error)

	// Install installs a chaincode package on the network.
	Install(ctx context.Context, pkg []byte) error
}
