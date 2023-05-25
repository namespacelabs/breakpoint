package main

import (
	"errors"
	"net"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"inet.af/tcpproxy"
	"namespacelabs.dev/breakpoint/pkg/quicproxyclient"
)

func newAttachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "attach",
	}

	endpoint := cmd.Flags().String("endpoint", "", "The address of the server.")
	target := cmd.Flags().String("target", "", "Where to connect to.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if *endpoint == "" {
			return errors.New("--endpoint is required")
		}

		if *target == "" {
			return errors.New("--target is required")
		}

		return quicproxyclient.Serve(cmd.Context(), *endpoint, nil, quicproxyclient.Handlers{
			OnAllocation: func(endpoint string) {
				zerolog.Ctx(cmd.Context()).Info().Str("endpoint", endpoint).Msg("Got allocation")
			},
			Proxy: func(conn net.Conn) error {
				zerolog.Ctx(cmd.Context()).Info().Str("target", *target).Msg("handling reverse proxy")
				go tcpproxy.To(*target).HandleConn(conn)
				return nil
			},
		})
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(newAttachCmd())
}
