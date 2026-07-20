package cmd

import "fmt"

// errNotImplemented is returned by subcommands that are scaffolded but whose
// behaviour has not been wired up yet.
func errNotImplemented(name string) error {
	return fmt.Errorf("%q is not implemented yet", name)
}
