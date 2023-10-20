package orderer

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/matcher"
	ordPb "github.com/hyperledger/fabric-protos-go/orderer"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type grpcPool struct {
	ctx context.Context
	l   *zap.Logger

	connMap map[string]*grpc.ClientConn
	connMx  sync.RWMutex

	tlsConf *tls.Config
	matcher *matcher.Matcher
}

func (p *grpcPool) Get(orderer *Orderer) (ordPb.AtomicBroadcastClient, error) {
	key := p.getOrdererMapKey(orderer)
	p.connMx.RLock()
	conn, ok := p.connMap[key]
	if ok {
		p.connMx.RUnlock()
		return ordPb.NewAtomicBroadcastClient(conn), nil
	}
	p.connMx.RUnlock()
	p.connMx.Lock()
	defer p.connMx.Unlock()
	// check connection again, can be set in another lock
	conn, ok = p.connMap[key]
	if ok {
		return ordPb.NewAtomicBroadcastClient(conn), nil
	}
	tlsConf := p.tlsConf.Clone()
	for _, cert := range orderer.Certificates {
		if !tlsConf.RootCAs.AppendCertsFromPEM(cert) {
			return nil, fmt.Errorf("failed to add cert: %s", string(cert))
		}
	}
	conn, err := grpc.Dial(key, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}
	p.connMap[key] = conn
	return ordPb.NewAtomicBroadcastClient(conn), nil
}

func (p *grpcPool) getOrdererMapKey(orderer *Orderer) string {
	v := fmt.Sprintf("%s:%d", orderer.Host, orderer.Port)
	if v, err := p.matcher.Match(v); err == nil {
		return v
	}
	return v
}

func (p *grpcPool) Close() error {
	p.connMx.Lock()
	defer p.connMx.Unlock()
	var result error
	for key, conn := range p.connMap {
		p.l.Debug("closing connection", zap.String("orderer", conn.Target()))
		if err := conn.Close(); err != nil {
			result = multierr.Append(result, err)
		}
		delete(p.connMap, key)
	}
	return result
}

func NewGrpcPool(ctx context.Context, logger *zap.Logger, tlsConf *tls.Config, m *matcher.Matcher) Pool {
	return &grpcPool{ctx: ctx, l: logger.Named("orderer_pool"), connMap: make(map[string]*grpc.ClientConn), tlsConf: tlsConf, matcher: m}
}
