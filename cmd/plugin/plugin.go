package main

import (
	"os"

	"github.com/prequel-dev/preq/cmd/plugin/krew"
	"github.com/prequel-dev/preq/internal/pkg/sigs"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {

	ctx := sigs.InitSignals()

	streams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	krew.InitAndExecute(ctx, streams)
}
