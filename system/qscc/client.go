package qscc

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

type cli struct {
	cli pb.EndorserClient
	id  protoutil.Signer
}

func NewClient(enCli pb.EndorserClient, id protoutil.Signer) Client {
	return &cli{cli: enCli, id: id}
}
