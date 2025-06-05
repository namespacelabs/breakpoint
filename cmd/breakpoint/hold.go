package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
	"namespacelabs.dev/breakpoint/pkg/bcontrol"
	"namespacelabs.dev/breakpoint/pkg/waiter"
)

func init() {
	rootCmd.AddCommand(newHoldCmd())
}

func newHoldCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hold",
		Short: "Holds until a breakpoint is finished or for a certain amount of time.",
	}

	holdFor := cmd.Flags().Duration("for", time.Minute*30, "How much to extend the breakpoint by.")
	holdDuration := cmd.Flags().Duration("duration", 0, "Alias of --for")
	shouldHoldWhileConnected := cmd.Flags().Bool("while-connected", false, "Keep holding while there are active connections, even after duration has passed")
	cmd.MarkFlagsMutuallyExclusive("duration", "for", "while-connected")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		duration := *holdDuration
		if *holdDuration == 0 {
			duration = *holdFor
		}

		if *shouldHoldWhileConnected {
			return holdWhileConnected(cmd.Context())
		} else {
			return holdForDuration(cmd.Context(), duration)
		}
	}

	return cmd
}

func holdForDuration(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}

	status, err := getStatus(ctx)
	if err != nil {
		return err
	}
	waiter.PrintConnectionInfo(status.Endpoint, status.Expiration.AsTime(), os.Stderr)

	fmt.Printf("Holding until %s\n", humanize.Time(time.Now().Add(duration)))

	timer := time.NewTimer(duration)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func holdWhileConnected(ctx context.Context) error {
	clt, conn, err := bcontrol.Connect(ctx)
	if err != nil {
		return err
	}

	defer conn.Close()

	status, err := clt.Status(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}

	if status.GetNumConnections() < 1 {
		fmt.Printf("No active connections, exiting\n")
		return nil
	}

	waiter.PrintConnectionInfo(status.Endpoint, status.Expiration.AsTime(), os.Stderr)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	fmt.Printf("Waiting until breakpoint has no active connections\n")

	errCount := 0
	getNumConnections := func() (uint32, error) {
		status, err := clt.Status(ctx, &emptypb.Empty{})
		if err != nil {
			return 0, err
		}

		return status.GetNumConnections(), nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			numConnections, err := getNumConnections()
			if err != nil {
				errCount++
				if errCount > 5 {
					return fmt.Errorf("unable to fetch breakpoint status: %w", err)
				} else {
					fmt.Printf("unable to fetch breakpoint status, trying again\n")
				}
				continue
			}

			errCount = 0

			if numConnections > 0 {
				fmt.Printf("Active connections: %d, waiting\n", numConnections)
				continue
			}

			fmt.Printf("No active connections, exiting\n")
			return nil
		}
	}
}
