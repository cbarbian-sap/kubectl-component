package cmd

import (
	"context"
	"fmt"

	"github.com/sap/kubectl-component/pkg/component"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type deleteOptions struct {
	factory *Factory
	names   []string
}

func newDeleteCommand(factory *Factory) *cobra.Command {
	o := &deleteOptions{factory: factory}

	cmd := &cobra.Command{
		Use:     "delete NAME [NAME...]",
		Short:   "Delete one or many Components",
		Long:    "Delete one or many Components by name in the current namespace.",
		Aliases: []string{"del", "rm"},
		Args:    cobra.MinimumNArgs(1),
		Example: "  # Delete a single Component\n  kubectl component delete my-component\n\n  # Delete several Components\n  kubectl component delete comp-a comp-b comp-c",
		RunE: func(cmd *cobra.Command, args []string) error {
			o.names = args
			return o.run(cmd.Context())
		},
	}

	return cmd
}

func (o *deleteOptions) run(ctx context.Context) error {
	client, err := component.NewClientset(o.factory.ConfigFlags)
	if err != nil {
		return err
	}

	namespace, err := component.ResolveNamespace(o.factory.ConfigFlags)
	if err != nil {
		return err
	}

	components := client.CoreV1alpha1().Components(namespace)

	for _, name := range o.names {
		if err := components.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			return err
		}
		fmt.Fprintf(o.factory.Streams.Out, "component.core.cs.sap.com/%s deleted\n", name)
	}

	return nil
}
