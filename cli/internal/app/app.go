// Package app wires the dev commands together. Main is the whole CLI behind a
// Runner, so tests drive it exactly as a user would, against a fake machine.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/doctor"
	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// errFailed means the command already reported what went wrong; exit non-zero quietly.
var errFailed = errors.New("failed")

// Main runs dev with args and returns the process exit code.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer, r sys.Runner) int {
	root := &cobra.Command{
		Use:           "dev",
		Short:         "Set up and maintain this machine from the dev-environment repo",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(doctorCmd(r))

	err := root.ExecuteContext(ctx)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errFailed):
		return 1
	default:
		fmt.Fprintln(stderr, "dev:", err)
		return 1
	}
}

func doctorCmd(r sys.Runner) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that this machine matches its profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := doctor.ReadMachine(cmd.Context(), r)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "== profile: %s\n", m.Profile)
			if doctor.Run(cmd.Context(), r, doctor.Checks(m), cmd.OutOrStdout()) > 0 {
				return errFailed
			}
			return nil
		},
	}
}
