// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package hostinfo

import (
	"context"
	"regexp"
	"strconv"

	"github.com/coreos/go-systemd/v22/dbus"
)

var SystemdVersion = getSystemdVersion()

func getSystemdVersion() int {
	var reVersion = regexp.MustCompile(`[0-9][0-9][0-9]`)

	conn, err := dbus.NewWithContext(context.Background())
	if err != nil {
		return 0
	}
	defer conn.Close()

	version, err := conn.GetManagerProperty("Version")
	if err != nil {
		return 0
	}

	major := reVersion.FindString(version)
	if major == "" {
		return 0
	}

	ver, err := strconv.Atoi(major)
	if err != nil {
		return 0
	}

	return ver
}
