package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var ErrFailedToReleaseLock = errors.New("failed to release migration lock")

func newUnlockCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Release a stuck migration lock",
		Long:  "Forcefully removes the distributed migration lock document so a new migration run can proceed.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !force && !promptConfirmation(cmd, "WARNING: Forcefully releasing the lock can lead to race conditions if another instance is still running. Continue? [y/N]: ") {
				fmt.Fprintln(cmd.OutOrStdout(), "Operation cancelled.")
				return nil
			}

			engine, err := getEngine(cmd.Context())
			if err != nil {
				return err
			}

			if err := engine.ForceUnlock(cmd.Context()); err != nil {
				return fmt.Errorf("%w: %w", ErrFailedToReleaseLock, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "✅ Migration lock released.")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}
