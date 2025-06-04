package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
	v1 "namespacelabs.dev/breakpoint/api/private/v1"
	"namespacelabs.dev/breakpoint/pkg/bcontrol"
	"namespacelabs.dev/breakpoint/pkg/execbackground"
	"namespacelabs.dev/breakpoint/pkg/waiter"
)

func init() {
	rootCmd.AddCommand(newStartCmd())
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Starts breakpoint in the background",
	}

	configPath := cmd.Flags().String("config", "", "Path to the configuration file.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if *configPath == "" {
			return errors.New("--config is required")
		}

		procArgs := []string{"wait", "--config", *configPath}
		proc := exec.Command(os.Args[0], procArgs...)
		execbackground.SetCreateSession(proc)

		if err := proc.Start(); err != nil {
			return fmt.Errorf("failed to start background process: %w", err)
		}

		pid := proc.Process.Pid

		fmt.Fprintf(os.Stderr, "Breakpoint starting in background (PID: %d)\n", pid)

		status, err := waitForReady(cmd.Context(), 5*time.Second)
		if err != nil {
			_ = proc.Process.Kill()
			return err
		}

		if err := proc.Process.Release(); err != nil {
			return err
		}

		waiter.PrintConnectionInfo(status.Endpoint, status.GetExpiration().AsTime(), os.Stderr)

		return nil
	}

	return cmd
}

func waitForReady(ctx context.Context, timeoutDuration time.Duration) (*v1.StatusResponse, error) {
	// Check for file existence with timeout
	timeout := time.After(timeoutDuration)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-timeout:
			return nil, fmt.Errorf("breakpoint didn't start in time")

		case <-ticker.C:
			status, err := getStatus(ctx)
			if err != nil {
				continue
			}

			if status.GetEndpoint() != "" {
				return status, nil
			}
		}
	}
}

func getStatus(ctx context.Context) (*v1.StatusResponse, error) {
	clt, conn, err := bcontrol.Connect(ctx)
	if err != nil {
		return nil, err
	}

	defer conn.Close()

	status, err := clt.Status(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	return status, nil
}
