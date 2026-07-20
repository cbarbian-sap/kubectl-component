package cmd

import (
	"context"
	"fmt"

	"github.com/sap/kubectl-component/pkg/component"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newSuspendCommand(factory *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "suspend NAME [NAME...]",
		Short: "Suspend reconciliation of one or many Components",
		Long:  "Suspend reconciliation of one or many Components by setting spec.suspend to true.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setSuspend(cmd.Context(), factory, args, true)
		},
	}
}

// setSuspend patches spec.suspend on each named Component to the given value.
func setSuspend(ctx context.Context, factory *Factory, names []string, suspend bool) error {
	client, err := component.NewClientset(factory.ConfigFlags)
	if err != nil {
		return err
	}

	namespace, err := component.ResolveNamespace(factory.ConfigFlags)
	if err != nil {
		return err
	}

	components := client.CoreV1alpha1().Components(namespace)
	patch := []byte(fmt.Sprintf(`{"spec":{"suspend":%t}}`, suspend))

	verb := "suspended"
	if !suspend {
		verb = "unsuspended"
	}

	for _, name := range names {
		if _, err := components.Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: fieldManager}); err != nil {
			return err
		}
		fmt.Fprintf(factory.Streams.Out, "component.core.cs.sap.com/%s %s\n", name, verb)
	}

	return nil
}
