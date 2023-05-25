package bgrpc

import (
	"context"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"
)

func DialContext(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	unary, streaming := clientInterceptors()

	opts = append(opts,
		grpc.WithStreamInterceptor(grpc_middleware.ChainStreamClient(streaming...)),
		grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(unary...)),
	)

	return grpc.DialContext(ctx, target, opts...)
}

func clientInterceptors() ([]grpc.UnaryClientInterceptor, []grpc.StreamClientInterceptor) {
	return []grpc.UnaryClientInterceptor{
			grpc_prometheus.UnaryClientInterceptor,
		}, []grpc.StreamClientInterceptor{
			grpc_prometheus.StreamClientInterceptor,
		}
}
