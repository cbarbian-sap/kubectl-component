# kubectl-component

A [kubectl plugin](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)
for inspecting and managing `Component` resources (`core.cs.sap.com/v1alpha1`)
served by the [SAP component-operator](https://sap.github.io/component-operator/).

Once installed it is invoked as:

```sh
kubectl component <subcommand>
```

## Subcommands

| Command | Status | Description |
| --- | --- | --- |
| `get` | implemented | List Components or get one/many by name (`-A`, `-o yaml/json/...`). |
| `create` | implemented | Create one or many Components from a manifest (`-f`). |
| `delete` | implemented | Delete one or many Components by name. |
| `suspend` | stub | Suspend reconciliation (`spec.suspend=true`). |
| `unsuspend` | stub | Resume reconciliation (`spec.suspend=false`). |
| `reconcile` | stub | Trigger an immediate reconciliation. |
| `describe` | stub | Show detailed, human-readable Component information. |
| `analyze` | stub | Analyze Component health and dependent resources. |

All commands accept the standard kubectl connection flags
(`--kubeconfig`, `--context`, `--namespace`/`-n`, `--server`, etc.).

## Build

```sh
make build       # produces ./bin/kubectl-component
```

## Install

Place the built binary anywhere on your `PATH` with the name `kubectl-component`:

```sh
make build
sudo install ./bin/kubectl-component /usr/local/bin/kubectl-component
# or
go install github.com/sap/kubectl-component/cmd/kubectl-component@latest
```

Verify kubectl discovers it:

```sh
kubectl plugin list
kubectl component --help
```

## Examples

```sh
# List all Components in the current namespace
kubectl component get

# List Components across all namespaces
kubectl component get -A

# Get a single Component as YAML
kubectl component get my-component -o yaml

# Create Components from a manifest (or stdin with -f -)
kubectl component create -f component.yaml

# Delete one or many Components
kubectl component delete comp-a comp-b
```