// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	"context"

	"github.com/coreos/go-systemd/v22/dbus"
)

type systemdClient interface {
	connect() (systemdConnection, error)
}
type systemdConnection interface {
	Close()
	GetManagerProperty(string) (string, error)
	ListUnitsContext(ctx context.Context) ([]dbus.UnitStatus, error)
	ListUnitsByPatternsContext(ctx context.Context, states []string, patterns []string) ([]dbus.UnitStatus, error)
}

type systemdDBusClient struct{}

func (systemdDBusClient) connect() (systemdConnection, error) {
	return dbus.NewWithContext(context.Background())
}

func newSystemdDBusClient() *systemdDBusClient {
	return &systemdDBusClient{}
}
