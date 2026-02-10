package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"namespacelabs.dev/breakpoint/pkg/blog"
)

var rootCmd = &cobra.Command{
	Use:   "breakpoint",
	Short: `Add breakpoints to CI workflows.`,
}

func main() {
	// This is the only control we have available.
	os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")

	l := blog.New()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := rootCmd.ExecuteContext(l.WithContext(ctx))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
