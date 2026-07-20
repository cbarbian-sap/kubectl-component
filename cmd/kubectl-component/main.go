package main

import (
	"os"

	"github.com/sap/kubectl-component/pkg/cmd"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

func main() {
	streams := genericiooptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	root := cmd.NewRootCommand(streams)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
