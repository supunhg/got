// Command got is a Git-native developer operating layer.
//
// Copyright 2026 Supun Hewagamage. MIT License.

package main

import (
	"fmt"
	"os"

	"github.com/supunhg/got/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "got:", err)
		os.Exit(1)
	}
}
