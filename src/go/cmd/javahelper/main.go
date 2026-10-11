// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/java/privileged"
	"os"
)

func main() {
	if err := privileged.Run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "java-helper:", err)
		os.Exit(1)
	}
}
