package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	v1 "namespacelabs.dev/breakpoint/api/private/v1"
	"namespacelabs.dev/breakpoint/pkg/bcontrol"
	"namespacelabs.dev/breakpoint/pkg/waiter"
)

func init() {
	rootCmd.AddCommand(newHoldCmd())
}

const (
	extendBy = 30 * time.Second
)

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

	tickDuration := 5 * time.Second
	ticker := time.NewTicker(tickDuration)
	defer ticker.Stop()

	fmt.Printf("Waiting until breakpoint has no active connections\n")

	errCount := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			status, err := clt.Status(ctx, &emptypb.Empty{})
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

			expiration := status.GetExpiration().AsTime()
			if !expiration.IsZero() && time.Now().Add(2*tickDuration).After(expiration) {
				tryExtendBreakpoint(ctx, expiration, clt)
			}

			if status.GetNumConnections() > 0 {
				fmt.Printf("Active connections: %d, waiting\n", status.GetNumConnections())
				continue
			}

			fmt.Printf("No active connections, exiting\n")
			return nil
		}
	}
}

func tryExtendBreakpoint(ctx context.Context, currentExpiration time.Time, clt v1.ControlServiceClient) {
	fmt.Printf("Breakpoint expiring %s, extending by %s\n", humanize.Time(currentExpiration), extendBy)

	ret, err := clt.Extend(ctx, &v1.ExtendRequest{
		WaitFor: durationpb.New(extendBy),
	})
	if err != nil {
		fmt.Printf("Unable to extend breakpoint, %v", err)
	}

	fmt.Printf("Breakpoint now expires %s", humanize.Time(ret.GetExpiration().AsTime()))
}
