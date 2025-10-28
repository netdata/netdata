//go:build !cgo || !ibm_mq

package main

import (
	"os"

	"github.com/netdata/netdata/go/plugins/logger"
)

func main() {
	logger.Errorf("ibm.d.plugin: this binary was built without IBM MQ support (requires CGO and ibm_mq build tag)")
	os.Exit(1)
}
