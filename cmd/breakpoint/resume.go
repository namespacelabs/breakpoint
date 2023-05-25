package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
	"namespacelabs.dev/breakpoint/pkg/bcontrol"
)

func newResumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume the workflow execution.",
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clt, conn, err := bcontrol.Connect(cmd.Context())
		if err != nil {
			return err
		}

		defer conn.Close()

		if _, err := clt.Resume(cmd.Context(), &emptypb.Empty{}); err != nil {
			return err
		}

		fmt.Printf("Breakpoint removed, workflow resuming!\n")
		return nil
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(newResumeCmd())
}
