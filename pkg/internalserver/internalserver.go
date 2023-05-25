package internalserver

import (
	"context"
	"log"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "namespacelabs.dev/breakpoint/api/private/v1"
	"namespacelabs.dev/breakpoint/pkg/bcontrol"
	"namespacelabs.dev/breakpoint/pkg/waiter"
)

type waiterService struct {
	manager *waiter.Manager
	pb.UnimplementedControlServiceServer
}

func ListenAndServe(ctx context.Context, mgr *waiter.Manager) error {
	socketPath, err := bcontrol.SocketPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return err
	}

	_ = os.Remove(socketPath) // Remove any leftovers.

	defer func() {
		_ = os.Remove(socketPath)
	}()

	var d net.ListenConfig
	lis, err := d.Listen(ctx, "unix", socketPath)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterControlServiceServer(grpcServer, waiterService{
		manager: mgr,
	})

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		<-ctx.Done()
		grpcServer.Stop()
		return nil
	})

	eg.Go(func() error {
		return grpcServer.Serve(lis)
	})

	return eg.Wait()
}

func (g waiterService) Extend(ctx context.Context, req *pb.ExtendRequest) (*pb.ExtendResponse, error) {
	expiration := g.manager.ExtendWait(req.WaitFor.AsDuration())
	return &pb.ExtendResponse{
		Expiration: timestamppb.New(expiration),
	}, nil
}

func (g waiterService) Resume(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	g.manager.StopWait()
	return &emptypb.Empty{}, nil
}
