package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	coreruntime "github.com/sap/component-operator-runtime/pkg/component"
	componentv1alpha1 "github.com/sap/component-operator/api/v1alpha1"
	"github.com/sap/kubectl-component/pkg/component"
	"github.com/spf13/cobra"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

type createOptions struct {
	factory   *Factory
	filenames []string

	// Attributes for building a single Component from flags (mutually
	// exclusive with --filename).
	labels               map[string]string
	annotations          map[string]string
	targetNamespace      string
	targetName           string
	targetKubeConfig     string
	targetServiceAccount string
	source               string
	valuesFile           string
	valuesFrom           []string
	requeueInterval      time.Duration
	retryInterval        time.Duration
	reapplyInterval      time.Duration
	timeout              time.Duration
	sticky               bool
	dependsOn            []string
	dryRun               bool
}

// builderFlags lists the flags that drive flag-based Component creation; they
// are mutually exclusive with --filename.
var builderFlags = []string{
	"labels", "annotations",
	"target-namespace", "target-name", "target-kubeconfig", "target-serviceaccount",
	"source", "values", "valuesFrom",
	"requeueInterval", "retryInterval", "reapplyInterval", "timeout", "sticky",
	"depends-on",
}

func newCreateCommand(factory *Factory) *cobra.Command {
	o := &createOptions{factory: factory}

	cmd := &cobra.Command{
		Use:   "create (-f FILENAME | NAME --source SOURCE [flags])",
		Short: "Create one or many Components",
		Long: "Create one or many Components.\n\n" +
			"Either provide one or more manifests via --filename/-f, or create a single\n" +
			"Component by passing its NAME together with --source and optional attribute\n" +
			"flags. The two modes are mutually exclusive.",
		Example: "  # Create Components from a manifest file (or stdin with -f -)\n" +
			"  kubectl component create -f component.yaml\n\n" +
			"  # Create a single Component from flags\n" +
			"  kubectl component create my-component --source git/flux-system/my-repo \\\n" +
			"    --target-namespace my-ns --values ./values.yaml --timeout 5m",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.run(cmd, cmd.Context(), args)
		},
	}

	cmd.Flags().StringSliceVarP(&o.filenames, "filename", "f", nil, "Path to a manifest file, or \"-\" for stdin (repeatable)")

	cmd.Flags().StringToStringVar(&o.labels, "labels", nil, "Labels to set on metadata.labels (key=value,...)")
	cmd.Flags().StringToStringVar(&o.annotations, "annotations", nil, "Annotations to set on metadata.annotations (key=value,...)")
	cmd.Flags().StringVar(&o.targetNamespace, "target-namespace", "", "Target deployment namespace (spec.namespace)")
	cmd.Flags().StringVar(&o.targetName, "target-name", "", "Target deployment name (spec.name)")
	cmd.Flags().StringVar(&o.targetKubeConfig, "target-kubeconfig", "", "Name of a secret holding the kubeconfig for a remote target cluster (spec.kubeConfig)")
	cmd.Flags().StringVar(&o.targetServiceAccount, "target-serviceaccount", "", "Service account to impersonate during reconciliation (spec.serviceAccountName)")
	cmd.Flags().StringVar(&o.source, "source", "", "Manifest source of the form (git|chart|oci|bucket|blueprint)/[namespace/]name")
	cmd.Flags().StringVar(&o.valuesFile, "values", "", "Path to a JSON or YAML file with templating values (spec.values)")
	cmd.Flags().StringArrayVar(&o.valuesFrom, "valuesFrom", nil, "Name of a secret containing values; repeatable (spec.valuesFrom)")
	cmd.Flags().DurationVar(&o.requeueInterval, "requeueInterval", 0, "Requeue interval after a successful reconciliation (spec.requeueInterval)")
	cmd.Flags().DurationVar(&o.retryInterval, "retryInterval", 0, "Retry interval after a retriable error (spec.retryInterval)")
	cmd.Flags().DurationVar(&o.reapplyInterval, "reapplyInterval", 0, "Force reapply interval (spec.reapplyInterval)")
	cmd.Flags().DurationVar(&o.timeout, "timeout", 0, "How long dependent objects have to become ready (spec.timeout)")
	cmd.Flags().BoolVar(&o.sticky, "sticky", false, "Stick to the source revision until ready or timeout (spec.sticky)")
	cmd.Flags().StringArrayVar(&o.dependsOn, "depends-on", nil, "Dependency of the form [namespace/]name; repeatable (spec.dependencies)")

	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "Do not submit anything to the API server; print the computed Component(s) as YAML instead")

	return cmd
}

func (o *createOptions) run(cmd *cobra.Command, ctx context.Context, args []string) error {
	if len(o.filenames) > 0 {
		if len(args) > 0 {
			return fmt.Errorf("a NAME argument cannot be combined with --filename/-f")
		}
		if changed := changedFlags(cmd, builderFlags); len(changed) > 0 {
			return fmt.Errorf("flags [%s] cannot be combined with --filename/-f", strings.Join(changed, ", "))
		}
		return o.runFromFile(ctx)
	}

	if len(args) != 1 {
		return fmt.Errorf("exactly one NAME argument is required unless --filename/-f is used")
	}
	if o.source == "" {
		return fmt.Errorf("--source is required when creating a Component from flags")
	}
	return o.runFromFlags(cmd, ctx, args[0])
}

// changedFlags returns the subset of names whose flag was explicitly set.
func changedFlags(cmd *cobra.Command, names []string) []string {
	var changed []string
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			changed = append(changed, name)
		}
	}
	return changed
}

func (o *createOptions) runFromFile(ctx context.Context) error {
	defaultNamespace, err := component.ResolveNamespace(o.factory.ConfigFlags)
	if err != nil {
		return err
	}

	objects, err := o.readManifests()
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return fmt.Errorf("no Component manifests found in the provided input")
	}

	for i := range objects {
		obj := &objects[i]
		if obj.Kind != "" && obj.Kind != component.GroupVersionKind.Kind {
			return fmt.Errorf("expected kind %q but got %q", component.GroupVersionKind.Kind, obj.Kind)
		}

		if obj.GetNamespace() == "" {
			obj.SetNamespace(defaultNamespace)
		}
	}

	if o.dryRun {
		for i := range objects {
			objects[i].SetGroupVersionKind(component.GroupVersionKind)
			if err := o.printComponentYAML(&objects[i]); err != nil {
				return err
			}
		}
		return nil
	}

	client, err := component.NewClientset(o.factory.ConfigFlags)
	if err != nil {
		return err
	}

	for i := range objects {
		obj := &objects[i]
		created, err := client.CoreV1alpha1().Components(obj.GetNamespace()).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		fmt.Fprintf(o.factory.Streams.Out, "component.core.cs.sap.com/%s created\n", created.GetName())
	}

	return nil
}

// runFromFlags builds a single Component from the attribute flags and creates it.
func (o *createOptions) runFromFlags(cmd *cobra.Command, ctx context.Context, name string) error {
	namespace, err := component.ResolveNamespace(o.factory.ConfigFlags)
	if err != nil {
		return err
	}

	sourceRef, err := parseSource(o.source)
	if err != nil {
		return err
	}

	comp := &componentv1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      o.labels,
			Annotations: o.annotations,
		},
	}
	comp.SetGroupVersionKind(component.GroupVersionKind)

	comp.Spec.SourceRef = sourceRef
	comp.Spec.Namespace = o.targetNamespace
	comp.Spec.Name = o.targetName
	comp.Spec.ServiceAccountName = o.targetServiceAccount
	comp.Spec.Sticky = o.sticky

	if o.targetKubeConfig != "" {
		comp.Spec.KubeConfig = &coreruntime.KubeConfigSpec{
			SecretRef: coreruntime.SecretKeyReference{Name: o.targetKubeConfig},
		}
	}

	if cmd.Flags().Changed("requeueInterval") {
		comp.Spec.RequeueInterval = &metav1.Duration{Duration: o.requeueInterval}
	}
	if cmd.Flags().Changed("retryInterval") {
		comp.Spec.RetryInterval = &metav1.Duration{Duration: o.retryInterval}
	}
	if cmd.Flags().Changed("reapplyInterval") {
		comp.Spec.ReapplyInterval = &metav1.Duration{Duration: o.reapplyInterval}
	}
	if cmd.Flags().Changed("timeout") {
		comp.Spec.Timeout = &metav1.Duration{Duration: o.timeout}
	}

	for _, secret := range o.valuesFrom {
		comp.Spec.ValuesFrom = append(comp.Spec.ValuesFrom, coreruntime.SecretKeyReference{Name: secret})
	}

	for _, dep := range o.dependsOn {
		nn, err := parseNamespacedName(dep)
		if err != nil {
			return fmt.Errorf("invalid --depends-on %q: %w", dep, err)
		}
		comp.Spec.Dependencies = append(comp.Spec.Dependencies, componentv1alpha1.Dependency{NamespacedName: nn})
	}

	if o.valuesFile != "" {
		values, err := readValuesFile(o.valuesFile)
		if err != nil {
			return err
		}
		comp.Spec.Values = values
	}

	if o.dryRun {
		return o.printComponentYAML(comp)
	}

	client, err := component.NewClientset(o.factory.ConfigFlags)
	if err != nil {
		return err
	}
	created, err := client.CoreV1alpha1().Components(namespace).Create(ctx, comp, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintf(o.factory.Streams.Out, "component.core.cs.sap.com/%s created\n", created.GetName())
	return nil
}

// printComponentYAML writes the given Component to stdout as a YAML document.
func (o *createOptions) printComponentYAML(comp *componentv1alpha1.Component) error {
	data, err := sigsyaml.Marshal(comp)
	if err != nil {
		return err
	}
	fmt.Fprintf(o.factory.Streams.Out, "---\n%s", data)
	return nil
}

// parseSource parses a --source value of the form
// (git|chart|oci|bucket|blueprint)/[namespace/]name into a SourceReference.
func parseSource(source string) (componentv1alpha1.SourceReference, error) {
	var ref componentv1alpha1.SourceReference

	parts := strings.Split(source, "/")
	var kind string
	var nn componentv1alpha1.NamespacedName
	switch len(parts) {
	case 2:
		kind = parts[0]
		nn.Name = parts[1]
	case 3:
		kind = parts[0]
		nn.Namespace = parts[1]
		nn.Name = parts[2]
	default:
		return ref, fmt.Errorf("invalid --source %q: expected (git|chart|oci|bucket|blueprint)/[namespace/]name", source)
	}
	if nn.Name == "" {
		return ref, fmt.Errorf("invalid --source %q: name must not be empty", source)
	}

	switch kind {
	case "git":
		ref.FluxGitRepository = &componentv1alpha1.FluxGitRepositoryReference{NamespacedName: nn}
	case "chart":
		ref.FluxHelmChart = &componentv1alpha1.FluxHelmChartReference{NamespacedName: nn}
	case "oci":
		ref.FluxOciRepository = &componentv1alpha1.FluxOciRepositoryReference{NamespacedName: nn}
	case "bucket":
		ref.FluxBucket = &componentv1alpha1.FluxBucketReference{NamespacedName: nn}
	case "blueprint":
		ref.Blueprint = &componentv1alpha1.BlueprintReference{NamespacedName: nn}
	default:
		return ref, fmt.Errorf("invalid --source %q: unknown source type %q (want git, chart, oci, bucket or blueprint)", source, kind)
	}

	return ref, nil
}

// parseNamespacedName parses a "[namespace/]name" value.
func parseNamespacedName(value string) (componentv1alpha1.NamespacedName, error) {
	var nn componentv1alpha1.NamespacedName
	parts := strings.Split(value, "/")
	switch len(parts) {
	case 1:
		nn.Name = parts[0]
	case 2:
		nn.Namespace = parts[0]
		nn.Name = parts[1]
	default:
		return nn, fmt.Errorf("expected [namespace/]name")
	}
	if nn.Name == "" {
		return nn, fmt.Errorf("name must not be empty")
	}
	return nn, nil
}

// readValuesFile reads a JSON or YAML file and returns it as JSON for spec.values.
func readValuesFile(path string) (*apiextensionsv1.JSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	jsonData, err := utilyaml.ToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse values file %s: %w", path, err)
	}
	return &apiextensionsv1.JSON{Raw: jsonData}, nil
}

// readManifests decodes all documents from the configured filenames (or stdin)
// into typed Component objects.
func (o *createOptions) readManifests() ([]componentv1alpha1.Component, error) {
	var objects []componentv1alpha1.Component

	for _, filename := range o.filenames {
		reader, closeReader, err := o.open(filename)
		if err != nil {
			return nil, err
		}

		decoder := utilyaml.NewYAMLOrJSONDecoder(reader, 4096)
		for {
			var obj componentv1alpha1.Component
			if err := decoder.Decode(&obj); err != nil {
				if err == io.EOF {
					break
				}
				closeReader()
				return nil, fmt.Errorf("failed to decode %s: %w", filename, err)
			}
			if obj.Kind == "" && obj.GetName() == "" {
				continue
			}
			objects = append(objects, obj)
		}
		closeReader()
	}

	// Guard against manifests that carry server-populated fields, which the API
	// server rejects on create.
	for i := range objects {
		objects[i].ResourceVersion = ""
		objects[i].UID = ""
		objects[i].CreationTimestamp = metav1.Time{}
	}

	return objects, nil
}

func (o *createOptions) open(filename string) (io.Reader, func(), error) {
	if filename == "-" {
		return o.factory.Streams.In, func() {}, nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	return file, func() { _ = file.Close() }, nil
}
