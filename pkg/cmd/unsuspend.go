package cmd

import "github.com/spf13/cobra"

func newUnsuspendCommand(factory *Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "unsuspend NAME [NAME...]",
		Short:   "Resume reconciliation of one or many Components",
		Long:    "Resume reconciliation of one or many Components by setting spec.suspend to false.",
		Aliases: []string{"resume"},
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setSuspend(cmd.Context(), factory, args, false)
		},
	}
}
