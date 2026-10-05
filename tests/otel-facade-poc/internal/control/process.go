// SPDX-License-Identifier: GPL-3.0-or-later

package control

import (
	"errors"
	"os"
	"sync"
)

// OCB constructs provider and extension factories independently. One process
// owns the one stdin/stdout pair; only Retrieve initializes this bridge. Factory
// construction and Scheme probes must remain free of I/O and goroutines.
var process struct {
	sync.Mutex
	controller *Controller
}

func OpenProcess() (*Controller, error) {
	process.Lock()
	defer process.Unlock()
	if process.controller == nil {
		endpoint := os.Getenv("NETDATA_OTEL_POC_ENDPOINT")
		if endpoint == "" {
			endpoint = "127.0.0.1:4317"
		}
		controller, err := New(os.Stdin, os.Stdout, endpoint, os.Getenv("NETDATA_OTEL_POC_STATE_DIR"))
		if err != nil {
			return nil, err
		}
		process.controller = controller
	}
	return process.controller, nil
}

func Process() (*Controller, error) {
	process.Lock()
	defer process.Unlock()
	if process.controller == nil {
		return nil, errors.New("netdata extension requires --config=netdata:local")
	}
	return process.controller, nil
}
