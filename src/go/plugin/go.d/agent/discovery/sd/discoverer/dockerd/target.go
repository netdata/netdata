// SPDX-License-Identifier: GPL-3.0-or-later

package dockerd

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
)

type targetGroup struct {
	source  string
	targets []model.Target
}

func (g *targetGroup) Provider() string        { return "sd:docker" }
func (g *targetGroup) Source() string          { return g.source }
func (g *targetGroup) Targets() []model.Target { return g.targets }

type target struct {
	model.Base `hash:"ignore"`

	hash uint64

	ID            string
	Name          string
	Image         string
	Command       string
	Labels        map[string]any
	PrivatePort   string // Port on the container
	PublicPort    string // Port exposed on the host
	PublicPortIP  string // Host IP address that the container's port is mapped to
	PortProtocol  string
	NetworkMode   string
	NetworkDriver string
	IPAddress     string

	Address string // "IPAddress:PrivatePort"
}

func (t *target) TUID() string {
	if t.PublicPort != "" {
		return fmt.Sprintf("%s_%s_%s_%s_%s_%s",
			t.Name, t.IPAddress, t.PublicPortIP, t.PortProtocol, t.PublicPort, t.PrivatePort)
	}
	if t.PrivatePort != "" {
		return fmt.Sprintf("%s_%s_%s_%s",
			t.Name, t.IPAddress, t.PortProtocol, t.PrivatePort)
	}
	return fmt.Sprintf("%s_%s", t.Name, t.IPAddress)
}

func (t *target) Hash() uint64 {
	return t.hash
}
