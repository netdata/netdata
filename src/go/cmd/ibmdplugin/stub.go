//go:build !cgo || !ibm_mq

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintf(os.Stderr, "ibm.d.plugin: this binary was built without IBM MQ support (requires CGO and ibm_mq build tag)\n")
	os.Exit(1)
}
