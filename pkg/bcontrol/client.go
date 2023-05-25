package bcontrol

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "namespacelabs.dev/breakpoint/api/private/v1"
	"namespacelabs.dev/breakpoint/pkg/bgrpc"
)

func SocketPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return dir, err
	}

	return filepath.Join(dir, "breakpoint/breakpoint.sock"), nil
}

func Connect(ctx context.Context) (pb.ControlServiceClient, *grpc.ClientConn, error) {
	socketPath, err := SocketPath()
	if err != nil {
		return nil, nil, err
	}

	conn, err := bgrpc.DialContext(ctx, socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		}))
	if err != nil {
		return nil, nil, err
	}

	return pb.NewControlServiceClient(conn), conn, nil
}
