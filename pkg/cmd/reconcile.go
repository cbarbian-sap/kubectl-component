package cmd

import "github.com/spf13/cobra"

func newReconcileCommand(factory *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "reconcile NAME [NAME...]",
		Short: "Trigger an immediate reconciliation of one or many Components",
		Long:  "Trigger an immediate reconciliation of one or many Components, typically by\nannotating them so the component-operator re-evaluates them without waiting\nfor the next requeue interval.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: annotate each named Component to force a reconcile.
			return errNotImplemented("reconcile")
		},
	}
}
