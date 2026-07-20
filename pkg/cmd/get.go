package cmd

import (
	"context"
	"fmt"
	"time"

	corestatus "github.com/sap/component-operator-runtime/pkg/status"
	componentv1alpha1 "github.com/sap/component-operator/api/v1alpha1"
	versioned "github.com/sap/component-operator/pkg/client/clientset/versioned"
	"github.com/sap/kubectl-component/pkg/component"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
)

type getOptions struct {
	factory       *Factory
	printFlags    *genericclioptions.PrintFlags
	allNamespaces bool
	selector      string
	noHeaders     bool
	names         []string
}

func newGetCommand(factory *Factory) *cobra.Command {
	o := &getOptions{
		factory:    factory,
		printFlags: genericclioptions.NewPrintFlags(""),
	}

	cmd := &cobra.Command{
		Use:     "get [NAME...]",
		Short:   "Display one or many Components",
		Long:    "Display one or many Components.\n\nWith no arguments all Components in the current namespace are listed.\nProvide one or more names to fetch specific Components.",
		Aliases: []string{"list", "ls"},
		Example: "  # List all Components in the current namespace\n  kubectl component get\n\n  # List Components across all namespaces\n  kubectl component get -A\n\n  # Get a single Component as YAML\n  kubectl component get my-component -o yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			o.names = args
			return o.run(cmd.Context())
		},
	}

	cmd.Flags().BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "List Components across all namespaces")
	cmd.Flags().StringVarP(&o.selector, "selector", "l", "", "Selector (label query) to filter on, supports '=', '==', and '!=' (e.g. -l key1=value1,key2=value2)")
	cmd.Flags().BoolVar(&o.noHeaders, "no-headers", false, "When using the default output format, don't print headers")
	o.printFlags.AddFlags(cmd)

	return cmd
}

func (o *getOptions) run(ctx context.Context) error {
	client, err := component.NewClientset(o.factory.ConfigFlags)
	if err != nil {
		return err
	}

	namespace, err := component.ResolveNamespace(o.factory.ConfigFlags)
	if err != nil {
		return err
	}

	items, err := o.collect(ctx, client, namespace)
	if err != nil {
		return err
	}

	// If an explicit output format was requested, delegate to the standard
	// cli-runtime printers (yaml, json, name, ...).
	if o.printFlags.OutputFormat != nil && *o.printFlags.OutputFormat != "" {
		printer, err := o.printFlags.ToPrinter()
		if err != nil {
			return err
		}
		for i := range items {
			// The typed client strips apiVersion/kind; restore it so the
			// printed manifests are complete and re-appliable.
			items[i].SetGroupVersionKind(component.GroupVersionKind)
			if err := printer.PrintObj(&items[i], o.factory.Streams.Out); err != nil {
				return err
			}
		}
		return nil
	}

	if len(items) == 0 {
		fmt.Fprintln(o.factory.Streams.ErrOut, "No Components found.")
		return nil
	}

	return o.printTable(items)
}

// collect returns the requested Components, either by listing them or by
// fetching the explicitly named ones.
func (o *getOptions) collect(ctx context.Context, client versioned.Interface, namespace string) ([]componentv1alpha1.Component, error) {
	if len(o.names) == 0 {
		listNamespace := namespace
		if o.allNamespaces {
			listNamespace = metav1.NamespaceAll
		}
		list, err := client.CoreV1alpha1().Components(listNamespace).List(ctx, metav1.ListOptions{LabelSelector: o.selector})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	}

	// Named lookups always target a concrete namespace.
	components := client.CoreV1alpha1().Components(namespace)
	items := make([]componentv1alpha1.Component, 0, len(o.names))
	for _, name := range o.names {
		obj, err := components.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		items = append(items, *obj)
	}
	return items, nil
}

func (o *getOptions) printTable(items []componentv1alpha1.Component) error {
	w := printers.GetNewTabWriter(o.factory.Streams.Out)
	defer w.Flush()

	if !o.noHeaders {
		if o.allNamespaces {
			fmt.Fprintln(w, "NAMESPACE\tNAME\tPHASE\tREASON\tOBJECTS\tREADY\tPROCESSING\tAGE")
		} else {
			fmt.Fprintln(w, "NAME\tPHASE\tREASON\tOBJECTS\tREADY\tPROCESSING\tAGE")
		}
	}

	for i := range items {
		item := &items[i]
		phase := string(item.Status.State)
		if phase == "" {
			phase = "<unknown>"
		}
		reason := readyReason(item)
		all, ready, processing := inventoryCounts(item)
		age := translateAge(item.GetCreationTimestamp())

		if o.allNamespaces {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%d\t%s\n", item.GetNamespace(), item.GetName(), phase, reason, all, ready, processing, age)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%d\t%s\n", item.GetName(), phase, reason, all, ready, processing, age)
		}
	}

	return nil
}

// readyReason reports the Reason of the Ready condition, or "<none>" if absent.
func readyReason(item *componentv1alpha1.Component) string {
	for _, condition := range item.Status.Conditions {
		if string(condition.Type) == "Ready" {
			if condition.Reason == "" {
				return "<none>"
			}
			return condition.Reason
		}
	}
	return "<none>"
}

// inventoryCounts returns the counts of all, ready, and processing objects in
// the status inventory. An item counts as ready when its phase is "Ready" and
// its status is Current, or when its phase is "Completed".
func inventoryCounts(item *componentv1alpha1.Component) (all, ready, processing int) {
	all = len(item.Status.Inventory)
	for _, inv := range item.Status.Inventory {
		if inv == nil {
			continue
		}
		if (inv.Phase == "Ready" && inv.Status == corestatus.CurrentStatus) || inv.Phase == "Completed" {
			ready++
		}
	}
	return all, ready, all - ready
}

// translateAge renders a creation timestamp as a compact human-readable age.
func translateAge(timestamp metav1.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}
	return duration(time.Since(timestamp.Time))
}

func duration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
