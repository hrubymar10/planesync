// Package main starts the planesync command.
package main

import (
	"os"

	"github.com/hrubymar10/planesync/cmd/planesync/internal/factory"
	"github.com/hrubymar10/planesync/internal/ui/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], factory.Build))
}
