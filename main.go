package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/flowchartsman/swaggerui"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/config"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/discovery"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/health"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/matcher"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/orderer"
	"github.com/atomyze-foundation/hlf-control-plane/pkg/peer"
	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/service/plane"
	srvMw "github.com/atomyze-foundation/hlf-control-plane/service/plane/middleware"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

var AppInfoVer string

var confFlag = flag.String("config", "config.yaml", "path to configuration file")

func main() {
	flag.Parse()

	ctx := context.Background()

	conf, err := config.Load(*confFlag)
	if err != nil {
		fmt.Println("load config: ", err)
	}
	// initialize logger
	lc := zap.NewProductionConfig()
	if err = lc.Level.UnmarshalText([]byte(conf.LogLevel)); err != nil {
		fmt.Println("parse log level:", err)
		os.Exit(1)
	}
	logger, err := lc.Build()
	if err != nil {
		fmt.Println("init logger:", err)
		os.Exit(1)
	}

	logger.Info("app version", zap.String("version", AppInfoVer))
	// read credentials for mutual tls
	tlsCreds, err := conf.TLS.TLSConfig()
	if err != nil {
		logger.Fatal("tls creds load failed", zap.Error(err))
	}
	// load signing identity
	id, err := conf.Identity.Load(logger, conf.MspID)
	if err != nil {
		logger.Fatal("identity load failed", zap.Error(err))
	}
	// create local peers instances
	localPeers := make([]*peer.Peer, 0, len(conf.Peers))
	for _, p := range conf.Peers {
		p.MspID = conf.MspID
		p.ClientCertificate = tlsCreds.Certificates[0]
		localPeers = append(localPeers, p)
	}
	// create host matcher for local development
	match := matcher.NewMatcher(conf.HostMatcher)
	// create peer grpc pool
	peerPool, err := peer.NewGrpcPool(ctx, logger, tlsCreds, localPeers, match)
	if err != nil {
		logger.Fatal("failed to init peer pool", zap.Error(err))
	}
	// create orderer grpc pool
	ordPool := orderer.NewGrpcPool(ctx, logger, tlsCreds, match)
	// create service instance
	ccSrv := plane.NewService(conf.MspID, id, logger, ordPool, peerPool, discovery.NewClient(logger, conf.MspID, peerPool, id), conf.Peers)
	// listen and server http and grpc servers
	if err = listen(ctx, logger, conf, ccSrv, peerPool, ordPool); err != nil {
		logger.Error("server returned error", zap.Error(err))
		os.Exit(1)
	}
}

func listen(ctx context.Context, logger *zap.Logger, conf *config.Config, ccSrv proto.ChaincodeServiceServer, pool peer.Pool, ordPool orderer.Pool) error { //nolint:funlen
	lis, err := net.Listen("tcp", conf.Listen.GRPC)
	if err != nil {
		logger.Fatal("bind grpc port failed", zap.Error(err), zap.String("port", conf.Listen.GRPC))
	}

	grpcServer := new(grpc.Server)
	httpServer := new(http.Server)

	// create new context with cancellation
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// listen system interrupt signals
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)

	// start err group with servers
	g, ctx := errgroup.WithContext(ctx)

	// start grpc server
	g.Go(func() error {
		grpcServer = grpc.NewServer(grpc.UnaryInterceptor(srvMw.AuthenticationInterceptor(conf.AccessToken)))
		proto.RegisterChaincodeServiceServer(grpcServer, ccSrv)
		grpc_health_v1.RegisterHealthServer(grpcServer, health.NewSrv())
		logger.Info("grpc listen", zap.String("port", conf.Listen.GRPC))
		return grpcServer.Serve(lis)
	})

	// start http server
	g.Go(func() error {
		conn, err := grpc.DialContext(ctx, conf.Listen.GRPC, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			return fmt.Errorf("grpc dial: %w", err)
		}

		mux := runtime.NewServeMux(srvMw.ErrorHandler(logger), runtime.WithHealthEndpointAt(grpc_health_v1.NewHealthClient(conn), "/v1/healthz"))
		if err = proto.RegisterChaincodeServiceHandler(ctx, mux, conn); err != nil {
			return fmt.Errorf("register gateway: %w", err)
		}

		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/v1") {
				mux.ServeHTTP(w, r)
				return
			}
			swaggerui.Handler(proto.SwaggerJSON).ServeHTTP(w, r)
		})

		httpServer = &http.Server{
			ReadHeaderTimeout: time.Second,
			Addr:              conf.Listen.HTTP,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				m := httpsnoop.CaptureMetrics(h, w, r)
				logger.Named("http").Info("request",
					zap.String("ip", r.RemoteAddr),
					zap.String("path", r.RequestURI),
					zap.Duration("duration", m.Duration),
					zap.Int("code", m.Code),
					zap.Int64("bytes", m.Written),
				)
			}),
		}
		logger.Info("http listen", zap.String("port", conf.Listen.HTTP))
		if err = httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	// block until context cancellation or interrupt signal
	select {
	case <-interrupt:
		break
	case <-ctx.Done():
		break
	}

	logger.Info("received shutdown signal")
	cancel()

	if err = httpServer.Shutdown(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			logger.Error("http server shutdown", zap.Error(err))
		}
	}
	grpcServer.GracefulStop()

	if err = g.Wait(); err != nil {
		return fmt.Errorf("wait error: %w", err)
	}

	logger.Info("waiting for peer pool close")
	if err = pool.Close(); err != nil {
		return fmt.Errorf("pool close: %w", err)
	}

	logger.Info("waiting for orderer pool close")
	if err = ordPool.Close(); err != nil {
		return fmt.Errorf("pool close: %w", err)
	}

	return nil
}
