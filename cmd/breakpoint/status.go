package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
	"namespacelabs.dev/breakpoint/pkg/bcontrol"
	"namespacelabs.dev/breakpoint/pkg/waiter"
)

func init() {
	rootCmd.AddCommand(newStatusCmd())
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Get the current status of breakpoint",
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clt, conn, err := bcontrol.Connect(cmd.Context())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stdout, "Unable to connect to breakpoint control server, is breakpoint running?")
			os.Exit(1)
			return nil
		}

		defer conn.Close()

		status, err := clt.Status(cmd.Context(), &emptypb.Empty{})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stdout, "Unable to retrieve status from breakpoint control server, is breakpoint running?")
			os.Exit(1)
			return nil
		}

		waiter.PrintConnectionInfo(status.Endpoint, status.Expiration.AsTime(), os.Stdout)

		return nil
	}

	return cmd
}
