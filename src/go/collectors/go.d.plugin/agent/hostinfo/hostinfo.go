// SPDX-License-Identifier: GPL-3.0-or-later

package hostinfo

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

var Hostname = getHostname()

func getHostname() string {
	path, err := exec.LookPath("hostname")
	if err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	bs, err := exec.CommandContext(ctx, path).Output()
	if err != nil {
		return ""
	}

	return string(bytes.TrimSpace(bs))
}
