package lifecycle

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

type cli struct {
	id  protoutil.Signer
	cli pb.EndorserClient
}

var _ Client = &cli{}

func NewClient(enCli pb.EndorserClient, id protoutil.Signer) Client {
	return &cli{cli: enCli, id: id}
}
