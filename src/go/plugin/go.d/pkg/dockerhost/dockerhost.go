// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhost

import (
	"fmt"
	"os"
	"strings"
)

func FromEnv() string {
	addr := os.Getenv("DOCKER_HOST")
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, "tcp://") || strings.HasPrefix(addr, "unix://") {
		return addr
	}
	if strings.HasPrefix(addr, "/") {
		return fmt.Sprintf("unix://%s", addr)
	}
	return fmt.Sprintf("tcp://%s", addr)
}
