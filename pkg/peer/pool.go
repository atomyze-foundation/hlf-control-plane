package peer

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"google.golang.org/grpc"
)

type Peer struct {
	Host              string
	Port              int32
	MspID             string
	CACertificates    [][]byte
	ClientCertificate tls.Certificate
}

func (p *Peer) String() string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

// Pool describes common interface for pool of peer clients
type Pool interface {
	// GetEndorser returns endorser client by peer definition
	GetEndorser(ctx context.Context, p *Peer) (pb.EndorserClient, error)
	// GetRandomEndorser returns random MSP endorser client by peer definition
	GetRandomEndorser(ctx context.Context, mspID string) (pb.EndorserClient, error)
	// GetDeliver returns deliver client by peer definition
	GetDeliver(ctx context.Context, p *Peer) (pb.DeliverClient, error)
	// GetConnection returns random grpc connection for certain MSP
	GetConnection(mspID string) (*grpc.ClientConn, []byte, error)
	// GetDiscoveryClient returns fabric discovery client
	GetDiscoveryClient(mspID string) (discovery.DiscoveryClient, error)
	// Close gracefully closes all pool connections
	Close() error
}
