package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
	pb "namespacelabs.dev/breakpoint/api/private/v1"
	"namespacelabs.dev/breakpoint/pkg/bcontrol"
	"namespacelabs.dev/breakpoint/pkg/waiter"

	"github.com/dustin/go-humanize"
)

func newExtendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extend",
		Short: "Extend the breakpoint duration.",
	}

	extendWaitFor := cmd.Flags().Duration("for", time.Minute*30, "How much to extend the breakpoint by.")
	extendWaitDuration := cmd.Flags().Duration("duration", 0, "Alias of --for")
	cmd.MarkFlagsMutuallyExclusive("duration", "for")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		duration := *extendWaitDuration
		if *extendWaitDuration == 0 {
			duration = *extendWaitFor
		}

		if duration <= 0 {
			return fmt.Errorf("duration must be positive")
		}

		clt, conn, err := bcontrol.Connect(cmd.Context())
		if err != nil {
			return err
		}

		defer conn.Close()

		resp, err := clt.Extend(cmd.Context(), &pb.ExtendRequest{
			WaitFor: durationpb.New(duration),
		})
		if err != nil {
			return err
		}

		expiration := resp.Expiration.AsTime()
		fmt.Printf("Breakpoint now expires at %s (%s)\n",
			expiration.Format(waiter.Stamp),
			humanize.Time(expiration))

		return nil
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(newExtendCmd())
}
