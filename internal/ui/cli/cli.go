// Package cli provides the non-interactive command-line interface.
package cli

import "fmt"

// Run executes the command with args and returns its exit code.
func Run(args []string) int {
	_ = args
	fmt.Println("Usage: planesync")
	return 0
}
