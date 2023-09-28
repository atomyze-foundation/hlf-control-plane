package plane

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	pb "github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/atomyze-foundation/hlf-control-plane/system/lifecycle"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *srv) ChaincodeInstall(ctx context.Context, request *pb.ChaincodeInstallRequest) (*pb.ChaincodeInstallResponse, error) { //nolint:funlen
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, request.Source, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "new request: %v", err)
	}
	s.logger.Debug("got auth headers", zap.Int("len", len(request.AuthHeaders)))
	for hk, hv := range request.AuthHeaders {
		req.Header.Add(hk, hv)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "do request: %v", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		s.logger.Error("expected 200 status, got another", zap.Int("code", resp.StatusCode))
		return nil, status.Errorf(codes.Internal, "http status 200 expected, got: %d", resp.StatusCode)
	}

	pkgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read pkg body: %v", err)
	}
	s.logger.Debug("fetched chaincode package", zap.Int("size", len(pkgBytes)))

	var wg sync.WaitGroup
	resChan := make(chan *pb.ChaincodeInstallResponse_Result)

	metadata, _, err := persistence.ParseChaincodePackage(pkgBytes)
	if err != nil {
		return nil, fmt.Errorf("parse package: %w", err)
	}
	s.logger.Debug("fetched package metadata", zap.String("label", metadata.Label), zap.String("path", metadata.Path))

	for _, p := range s.localPeers {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.logger.Debug("processing package on peer", zap.String("label", metadata.Label), zap.String("type", metadata.Type), zap.Int("size", len(pkgBytes)))
			endCli, err := s.peerPool.GetEndorser(ctx, p)
			if err != nil {
				resChan <- &pb.ChaincodeInstallResponse_Result{
					Peer:   p.String(),
					Result: &pb.ChaincodeInstallResponse_Result_Err{Err: fmt.Sprintf("get endorser: %s", err)},
				}
				return
			}
			s.processPeerInstall(ctx, metadata.Label, pkgBytes, p.String(), endCli, resChan)
		}()
	}

	result := make([]*pb.ChaincodeInstallResponse_Result, 0)
	go func() {
		wg.Wait()
		close(resChan)
	}()

	for res := range resChan {
		result = append(result, res)
	}

	return &pb.ChaincodeInstallResponse{Result: result}, nil
}

func (s *srv) processPeerInstall(ctx context.Context, label string, pkgBytes []byte, peerName string, endCli peer.EndorserClient, resChan chan<- *pb.ChaincodeInstallResponse_Result) {
	res := &pb.ChaincodeInstallResponse_Result{Peer: peerName, Label: label}
	defer func() {
		resChan <- res
	}()

	lcCli := lifecycle.NewClient(endCli, s.id)

	installed, err := lcCli.QueryInstalled(ctx)
	if err != nil {
		res.Result = &pb.ChaincodeInstallResponse_Result_Err{Err: fmt.Sprintf("query installed: %s", err)}
		return
	}

	for _, cc := range installed {
		if cc.Label == label {
			res.Result = &pb.ChaincodeInstallResponse_Result_Existed{Existed: true}
			return
		}
	}

	if err = lcCli.Install(ctx, pkgBytes); err != nil {
		res.Result = &pb.ChaincodeInstallResponse_Result_Err{Err: fmt.Sprintf("install: %s", err)}
		return
	}

	res.Result = &pb.ChaincodeInstallResponse_Result_Existed{Existed: false}
}
