package peer

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/matcher"
	"github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	cutil "github.com/hyperledger/fabric/common/util"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type grpcPool struct {
	connMap map[string]*grpc.ClientConn
	connMx  sync.RWMutex

	tlsConf *tls.Config
	l       *zap.Logger
}

func (p *grpcPool) GetEndorser(ctx context.Context, peer *Peer) (pb.EndorserClient, error) {
	key := p.getPeerMapKey(peer)
	p.connMx.RLock()
	conn, ok := p.connMap[key]
	if ok {
		p.connMx.RUnlock()
		p.l.Debug("found conn", zap.String("peer", peer.Host))
		return pb.NewEndorserClient(conn), nil
	}
	p.connMx.RUnlock()
	p.connMx.Lock()
	defer p.connMx.Unlock()
	// check connection again, can be set in another lock
	conn, ok = p.connMap[key]
	if ok {
		return pb.NewEndorserClient(conn), nil
	}
	var err error
	if conn, err = p.initGrpcConnection(ctx, peer); err != nil {
		return nil, fmt.Errorf("init grpc connection: %w", err)
	}
	p.connMap[key] = conn
	return pb.NewEndorserClient(conn), nil
}

func (p *grpcPool) GetRandomEndorser(_ context.Context, mspID string) (pb.EndorserClient, error) {
	p.connMx.RLock()
	defer p.connMx.RUnlock()
	p.l.Debug("searching for random peer", zap.String("mspID", mspID))
	for key, conn := range p.connMap {
		if strings.HasPrefix(key, mspID) {
			return pb.NewEndorserClient(conn), nil
		}
	}
	return nil, fmt.Errorf("no peers found")
}

func (p *grpcPool) GetDeliver(ctx context.Context, peer *Peer) (pb.DeliverClient, error) {
	key := p.getPeerMapKey(peer)
	p.connMx.RLock()
	conn, ok := p.connMap[key]
	if ok {
		p.connMx.RUnlock()
		return pb.NewDeliverClient(conn), nil
	}
	p.connMx.RUnlock()
	p.connMx.Lock()
	defer p.connMx.Unlock()
	// check connection again, can be set in another lock
	conn, ok = p.connMap[key]
	if ok {
		return pb.NewDeliverClient(conn), nil
	}
	p.l.Debug("new peer conn", zap.String("peer", peer.Host))
	tlsConf := p.tlsConf.Clone()
	for _, cert := range peer.CACertificates {
		if !tlsConf.RootCAs.AppendCertsFromPEM(cert) {
			return nil, fmt.Errorf("failed to add cert: %s", string(cert))
		}
	}
	conn, err := grpc.DialContext(ctx, fmt.Sprintf("%s:%d", peer.Host, peer.Port), grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}
	p.connMap[key] = conn
	return pb.NewDeliverClient(conn), nil
}

func (p *grpcPool) GetConnection(mspID string) (*grpc.ClientConn, []byte, error) {
	p.connMx.RLock()
	defer p.connMx.RUnlock()
	for key, conn := range p.connMap {
		if strings.HasPrefix(key, mspID) {
			var tlsCertHash []byte
			if p.tlsConf.Certificates[0].Certificate != nil {
				tlsCertHash = cutil.ComputeSHA256(p.tlsConf.Certificates[0].Certificate[0])
			}
			return conn, tlsCertHash, nil
		}
	}
	return nil, nil, fmt.Errorf("no connections found")
}

func (p *grpcPool) GetDiscoveryClient(mspID string) (discovery.DiscoveryClient, error) {
	p.connMx.RLock()
	defer p.connMx.RUnlock()
	for key, conn := range p.connMap {
		if strings.HasPrefix(key, mspID) {
			return discovery.NewDiscoveryClient(conn), nil
		}
	}
	return nil, fmt.Errorf("no peers found")
}

func (p *grpcPool) Close() error {
	p.connMx.Lock()
	defer p.connMx.Unlock()
	var result error
	for key, conn := range p.connMap {
		p.l.Debug("closing connection", zap.String("peer", conn.Target()))
		if err := conn.Close(); err != nil {
			result = multierr.Append(result, err)
		}
		delete(p.connMap, key)
	}
	return result
}

func (p *grpcPool) initGrpcConnection(ctx context.Context, peer *Peer) (*grpc.ClientConn, error) {
	p.l.Debug("new peer conn", zap.String("peer", peer.Host), zap.Int32("port", peer.Port))
	tlsConf := p.tlsConf.Clone()
	for _, cert := range peer.CACertificates {
		if !tlsConf.RootCAs.AppendCertsFromPEM(cert) {
			return nil, fmt.Errorf("failed to add cert: %s", string(cert))
		}
	}
	conn, err := grpc.DialContext(ctx, fmt.Sprintf("%s:%d", peer.Host, peer.Port), grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}
	p.l.Debug("connection initialized", zap.String("peer", peer.Host), zap.Int32("port", peer.Port))
	return conn, err
}

func (p *grpcPool) getPeerMapKey(peer *Peer) string {
	return fmt.Sprintf("%s_%s%d", peer.MspID, peer.Host, peer.Port)
}

func NewGrpcPool(ctx context.Context, logger *zap.Logger, tlsConf *tls.Config, localPeers []*Peer, _ *matcher.Matcher) (Pool, error) {
	pool := &grpcPool{connMap: make(map[string]*grpc.ClientConn), tlsConf: tlsConf, l: logger.Named("peer_pool")}
	for _, p := range localPeers {
		if _, err := pool.GetEndorser(ctx, p); err != nil {
			return nil, fmt.Errorf("peer `%s` init: %w", p.String(), err)
		}
	}
	return pool, nil
}
