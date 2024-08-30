// SPDX-License-Identifier: GPL-3.0-or-later

package netlisteners

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
)

type targetGroup struct {
	provider string
	source   string
	targets  []model.Target
}

func (g *targetGroup) Provider() string        { return g.provider }
func (g *targetGroup) Source() string          { return g.source }
func (g *targetGroup) Targets() []model.Target { return g.targets }

type target struct {
	model.Base

	hash uint64

	Protocol  string
	IPAddress string
	Port      string
	Comm      string
	Cmdline   string

	Address string // "IPAddress:Port"
}

func (t *target) TUID() string { return tuid(t) }
func (t *target) Hash() uint64 { return t.hash }

func tuid(tgt *target) string {
	return fmt.Sprintf("%s_%s_%d", strings.ToLower(tgt.Protocol), tgt.Port, tgt.hash)
}
