package middleware

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	authHeader = "authorization"
)

func AuthenticationInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		// skip auth check on service healthcheck
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "metadata is not provided")
		}
		if len(md.Get(authHeader)) != 1 {
			return nil, status.Errorf(codes.Unauthenticated, "auth header is not provided")
		}
		if md.Get(authHeader)[0] != token {
			return nil, status.Errorf(codes.Unauthenticated, "auth header is invalid")
		}
		return handler(ctx, req)
	}
}
