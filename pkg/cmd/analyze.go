package cmd

import "github.com/spf13/cobra"

func newAnalyzeCommand(factory *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "analyze NAME [NAME...]",
		Short: "Analyze the health and dependent resources of one or many Components",
		Long:  "Analyze one or many Components by inspecting their status, conditions and\nmanaged (dependent) resources to help diagnose why a Component is not ready.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: inspect status/inventory and report likely problems.
			return errNotImplemented("analyze")
		},
	}
}
