package cscc

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

type cli struct {
	cli pb.EndorserClient
	id  protoutil.Signer
}

// NewClient creates and returns a new endorser client for interacting with an endorser service.
func NewClient(enCli pb.EndorserClient, id protoutil.Signer) Client {
	return &cli{cli: enCli, id: id}
}
