package cmd

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	coreruntime "github.com/sap/component-operator-runtime/pkg/component"
	"github.com/sap/component-operator-runtime/pkg/reconciler"
	corestatus "github.com/sap/component-operator-runtime/pkg/status"
	componentv1alpha1 "github.com/sap/component-operator/api/v1alpha1"
	"github.com/sap/kubectl-component/pkg/component"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
)

func newDescribeCommand(factory *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "describe NAME [NAME...]",
		Short: "Show details of one or many Components",
		Long:  "Show a human-readable description of one or many Components, including their\nsource, target, configuration, reconciliation settings, policies, dependencies\nand recent events.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return describeComponents(cmd.Context(), factory, args)
		},
	}
}

func describeComponents(ctx context.Context, factory *Factory, names []string) error {
	client, err := component.NewClientset(factory.ConfigFlags)
	if err != nil {
		return err
	}
	kube, err := component.NewKubernetesClientset(factory.ConfigFlags)
	if err != nil {
		return err
	}
	namespace, err := component.ResolveNamespace(factory.ConfigFlags)
	if err != nil {
		return err
	}

	components := client.CoreV1alpha1().Components(namespace)
	for i, name := range names {
		comp, err := components.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if i > 0 {
			fmt.Fprintln(factory.Streams.Out)
		}
		if err := describeComponent(ctx, factory, kube, comp); err != nil {
			return err
		}
	}
	return nil
}

// sectionWriter formats aligned "key: value" lines and section headers.
type sectionWriter struct{ w *tabwriter.Writer }

func (s sectionWriter) title(t string)         { fmt.Fprintf(s.w, "\n%s:\n", t) }
func (s sectionWriter) kv(key, value string)   { fmt.Fprintf(s.w, "%s:\t%s\n", key, value) }
func (s sectionWriter) item(key, value string) { fmt.Fprintf(s.w, "  %s:\t%s\n", key, value) }
func (s sectionWriter) line(value string)      { fmt.Fprintf(s.w, "  %s\n", value) }
func (s sectionWriter) contValue(value string) { fmt.Fprintf(s.w, "\t%s\n", value) }

func describeComponent(ctx context.Context, factory *Factory, kube kubernetes.Interface, comp *componentv1alpha1.Component) error {
	out := factory.Streams.Out
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	s := sectionWriter{w: w}
	spec := &comp.Spec

	// Section 1: identity.
	s.kv("Name", comp.Name)
	s.kv("Namespace", comp.Namespace)
	writeMap(s, "Labels", comp.Labels)
	writeMap(s, "Annotations", filterAnnotations(comp.Annotations))

	// Section 2: source.
	s.title("Source")
	kind, ref := sourceSummary(spec.SourceRef)
	s.item("Type", kind)
	s.item("Reference", ref)
	if spec.Path != "" {
		s.item("Path", spec.Path)
	}
	if spec.Revision != "" {
		s.item("Pinned Revision", spec.Revision)
	}
	if spec.Digest != "" {
		s.item("Pinned Digest", spec.Digest)
	}
	if status := comp.Status.SourceRef; status != nil {
		s.item("Artifact URL", orNone(status.Artifact.Url))
		s.item("Artifact Revision", orNone(status.Artifact.Revision))
		s.item("Artifact Digest", orNone(status.Artifact.Digest))
	}

	// Section 3: target.
	s.title("Target")
	targetShown := false
	if spec.Namespace != "" {
		s.item("Namespace", spec.Namespace)
		targetShown = true
	}
	if spec.Name != "" {
		s.item("Name", spec.Name)
		targetShown = true
	}
	if spec.KubeConfig != nil {
		s.item("KubeConfig Secret", spec.KubeConfig.SecretRef.Name)
		targetShown = true
	}
	if spec.ServiceAccountName != "" {
		s.item("Service Account", spec.ServiceAccountName)
		targetShown = true
	}
	if !targetShown {
		s.line("<none> (deploys in-cluster to the Component's own namespace/name)")
	}

	// Section 4: configuration summary.
	s.title("Configuration")
	s.item("Values", valuesSummary(spec.Values))
	s.item("Values From", valuesFromSummary(spec.ValuesFrom))
	s.item("Decryption", decryptionSummary(spec.Decryption))
	s.item("Post-Build", postBuildSummary(spec.PostBuild))

	// Section 5: reconciliation.
	s.title("Reconciliation")
	s.item("Suspended", fmt.Sprintf("%t", spec.Suspend))
	s.item("Requeue Interval", durationOrNone(spec.RequeueInterval))
	s.item("Retry Interval", durationOrNone(spec.RetryInterval))
	s.item("Reapply Interval", durationOrNone(spec.ReapplyInterval))
	s.item("Timeout", durationOrNone(spec.Timeout))
	s.item("Sticky", fmt.Sprintf("%t", spec.Sticky))

	// Section 6: policies.
	s.title("Policies")
	s.item("Adoption", orDefault(string(spec.AdoptionPolicy)))
	s.item("Update", orDefault(string(spec.UpdatePolicy)))
	s.item("Delete", orDefault(string(spec.DeletePolicy)))
	s.item("Missing Namespaces", orDefault(string(spec.MissingNamespacesPolicy)))

	// Section 7: dependencies.
	s.title("Dependencies")
	if len(spec.Dependencies) == 0 {
		s.line("<none>")
	} else {
		for _, dep := range spec.Dependencies {
			s.line(dep.String())
		}
	}

	// Section 8: additional managed types.
	s.title("Additional Managed Types")
	if len(spec.AdditionalManagedTypes) == 0 {
		s.line("<none>")
	} else {
		for _, t := range spec.AdditionalManagedTypes {
			if t.Group == "" {
				s.line(t.Kind)
			} else {
				s.line(t.Group + "/" + t.Kind)
			}
		}
	}

	// Section: inventory insights.
	s.title("Inventory")
	inventory := comp.Status.Inventory
	unready := unreadyInventory(inventory)
	s.item("Objects", fmt.Sprintf("%d", len(inventory)))
	s.item("Unready", fmt.Sprintf("%d", len(unready)))
	for _, item := range unready {
		s.line(inventoryItemString(item))
	}

	if err := w.Flush(); err != nil {
		return err
	}

	// Section 9: events.
	return describeEvents(ctx, out, kube, comp)
}

func describeEvents(ctx context.Context, out io.Writer, kube kubernetes.Interface, comp *componentv1alpha1.Component) error {
	selector := fields.Set{
		"involvedObject.name":      comp.Name,
		"involvedObject.namespace": comp.Namespace,
	}.AsSelector().String()

	eventList, err := kube.CoreV1().Events(comp.Namespace).List(ctx, metav1.ListOptions{FieldSelector: selector})
	if err != nil {
		return err
	}

	events := make([]corev1.Event, 0, len(eventList.Items))
	for _, e := range eventList.Items {
		if e.InvolvedObject.Kind != component.GroupVersionKind.Kind {
			continue
		}
		if comp.UID != "" && e.InvolvedObject.UID != "" && e.InvolvedObject.UID != comp.UID {
			continue
		}
		events = append(events, e)
	}
	sort.Slice(events, func(i, j int) bool {
		return eventTimestamp(events[i]).Before(eventTimestamp(events[j]))
	})

	fmt.Fprintf(out, "\nEvents:\n")
	ew := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	defer ew.Flush()

	if len(events) == 0 {
		fmt.Fprintln(ew, "  <none>")
		return nil
	}

	fmt.Fprintln(ew, "  TYPE\tREASON\tAGE\tFROM\tMESSAGE")
	for _, e := range events {
		age := duration(time.Since(eventTimestamp(e)))
		if e.Count > 1 {
			age = fmt.Sprintf("%s (x%d)", age, e.Count)
		}
		fmt.Fprintf(ew, "  %s\t%s\t%s\t%s\t%s\n",
			orNone(e.Type), orNone(e.Reason), age, orNone(eventSource(e)), singleLine(e.Message))
	}
	return nil
}

// filterAnnotations returns a copy of the annotations without noisy,
// tool-managed entries such as the last-applied-configuration.
func filterAnnotations(annotations map[string]string) map[string]string {
	if len(annotations) == 0 {
		return annotations
	}
	filtered := make(map[string]string, len(annotations))
	for k, v := range annotations {
		if k == corev1.LastAppliedConfigAnnotation {
			continue
		}
		filtered[k] = v
	}
	return filtered
}

// writeMap prints a map as a top-level key with aligned, sorted continuation lines.
func writeMap(s sectionWriter, key string, m map[string]string) {
	if len(m) == 0 {
		s.kv(key, "<none>")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		entry := fmt.Sprintf("%s=%s", k, m[k])
		if i == 0 {
			s.kv(key, entry)
		} else {
			s.contValue(entry)
		}
	}
}

// sourceSummary returns the source type and its reference (namespace/name or URL).
func sourceSummary(ref componentv1alpha1.SourceReference) (string, string) {
	switch {
	case ref.Blueprint != nil:
		return "blueprint", ref.Blueprint.String()
	case ref.HttpRepository != nil:
		return "httpRepository", ref.HttpRepository.Url
	case ref.FluxGitRepository != nil:
		return "fluxGitRepository", ref.FluxGitRepository.String()
	case ref.FluxOciRepository != nil:
		return "fluxOciRepository", ref.FluxOciRepository.String()
	case ref.FluxBucket != nil:
		return "fluxBucket", ref.FluxBucket.String()
	case ref.FluxHelmChart != nil:
		return "fluxHelmChart", ref.FluxHelmChart.String()
	default:
		return "<none>", "<none>"
	}
}

func valuesSummary(values *apiextensionsv1.JSON) string {
	if values == nil {
		return "<none>"
	}
	return fmt.Sprintf("inline (%d bytes)", len(values.Raw))
}

func valuesFromSummary(refs []coreruntime.SecretKeyReference) string {
	if len(refs) == 0 {
		return "<none>"
	}
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, r.Name)
	}
	return strings.Join(names, ", ")
}

func decryptionSummary(d *componentv1alpha1.Decryption) string {
	if d == nil {
		return "<none>"
	}
	provider := d.Provider
	if provider == "" {
		provider = "sops"
	}
	return fmt.Sprintf("%s (secret: %s)", provider, d.SecretRef.Name)
}

func postBuildSummary(pb *componentv1alpha1.PostBuild) string {
	if pb == nil {
		return "<none>"
	}
	return fmt.Sprintf("substitute=%d, substituteFrom=%d, patches=%d, images=%d",
		len(pb.Substitute), len(pb.SubstituteFrom), len(pb.Patches), len(pb.Images))
}

func orNone(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}

func orDefault(s string) string {
	if s == "" {
		return "<default>"
	}
	return s
}

func durationOrNone(d *metav1.Duration) string {
	if d == nil {
		return "<none>"
	}
	return d.Duration.String()
}

func eventTimestamp(e corev1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	return e.CreationTimestamp.Time
}

func eventSource(e corev1.Event) string {
	if e.Source.Component != "" {
		return e.Source.Component
	}
	return e.ReportingController
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// unreadyInventory returns the inventory items that are not ready, i.e. whose
// phase is not "Ready" or whose status is not Current.
func unreadyInventory(items []*reconciler.InventoryItem) []*reconciler.InventoryItem {
	var unready []*reconciler.InventoryItem
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.Phase != reconciler.PhaseReady || item.Status != corestatus.CurrentStatus {
			unready = append(unready, item)
		}
	}
	return unready
}

// inventoryItemString renders an inventory item as a readable identity with its
// phase and status.
func inventoryItemString(item *reconciler.InventoryItem) string {
	kind := item.Kind
	if item.Group != "" {
		kind = item.Kind + "." + item.Group
	}
	name := item.Name
	if item.Namespace != "" {
		name = item.Namespace + "/" + item.Name
	}
	phase := string(item.Phase)
	if phase == "" {
		phase = "<none>"
	}
	return fmt.Sprintf("%s %s (phase=%s, status=%s)", kind, name, phase, item.Status.String())
}
