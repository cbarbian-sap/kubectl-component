// Package cmd implements the cobra command tree for the kubectl-component
// plugin. The plugin is invoked as `kubectl component <subcommand>`.
package cmd

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

// Factory bundles the shared dependencies that every subcommand needs: the
// standard kubectl connection flags and the input/output streams.
type Factory struct {
	ConfigFlags *genericclioptions.ConfigFlags
	Streams     genericiooptions.IOStreams
}

// fieldManager is the field manager used for all change operations (patches,
// applies) issued by this plugin.
const fieldManager = "component-client.cs.sap.com"

// NewRootCommand builds the root `kubectl component` command and wires up all
// subcommands.
func NewRootCommand(streams genericiooptions.IOStreams) *cobra.Command {
	factory := &Factory{
		ConfigFlags: genericclioptions.NewConfigFlags(true),
		Streams:     streams,
	}

	root := &cobra.Command{
		Use:          "kubectl-component",
		Short:        "Manage SAP component-operator Components",
		Long:         "kubectl-component is a kubectl plugin for inspecting and managing\nComponent resources (core.cs.sap.com/v1alpha1) served by the SAP\ncomponent-operator.",
		SilenceUsage: true,
	}

	// Register the standard kubectl connection flags (--kubeconfig, --context,
	// --namespace, etc.) as persistent flags on the root command.
	factory.ConfigFlags.AddFlags(root.PersistentFlags())

	root.AddCommand(
		newGetCommand(factory),
		newCreateCommand(factory),
		newDeleteCommand(factory),
		newSuspendCommand(factory),
		newUnsuspendCommand(factory),
		newReconcileCommand(factory),
		newDescribeCommand(factory),
		newAnalyzeCommand(factory),
	)

	return root
}
