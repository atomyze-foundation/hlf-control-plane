package plane

import (
	sd "github.com/atomyze-foundation/hlf-control-plane/pkg/delivery"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/discovery"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/orderer"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/peer"
	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/hyperledger/fabric/protoutil"
	"go.uber.org/zap"
)

// check server interface during compilation
var _ proto.ChaincodeServiceServer = &srv{}

type srv struct {
	mspID      string
	localPeers []*peer.Peer
	logger     *zap.Logger
	ordPool    orderer.Pool
	peerPool   peer.Pool
	id         protoutil.Signer
	dCli       sd.Client
	discCli    discovery.Client

	proto.UnimplementedChaincodeServiceServer
}

// NewService creates and returns a new instance of the ChaincodeServiceServer,
// which serves as a server for interacting with chaincode-related services.
func NewService(
	mspID string,
	signer protoutil.Signer,
	logger *zap.Logger,
	ordPool orderer.Pool,
	peerPool peer.Pool,
	discoveryCli discovery.Client,
	localPeers []*peer.Peer,
) proto.ChaincodeServiceServer {
	return &srv{
		mspID:      mspID,
		localPeers: localPeers,
		logger:     logger.Named("plane").With(zap.String("mspId", mspID)),
		ordPool:    ordPool,
		id:         signer,
		peerPool:   peerPool,
		discCli:    discoveryCli,
		dCli:       sd.NewPeer(logger, peerPool, localPeers, signer),
	}
}
