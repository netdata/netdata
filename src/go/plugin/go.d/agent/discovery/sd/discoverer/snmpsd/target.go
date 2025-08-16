// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"fmt"
	"sync"

	"github.com/gohugoio/hashstructure"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func targetSource(sub subnet) string { return fmt.Sprintf("discoverer=snmp,network=%s", subKey(sub)) }

func newTargetGroup(sub subnet) *targetGroup {
	return &targetGroup{
		provider: "sd:snmpdiscoverer",
		source:   targetSource(sub),
	}
}

type targetGroup struct {
	provider string
	source   string
	mux      sync.Mutex
	targets  []model.Target
}

func (g *targetGroup) Provider() string        { return g.provider }
func (g *targetGroup) Source() string          { return g.source }
func (g *targetGroup) Targets() []model.Target { return g.targets }

func (g *targetGroup) addTarget(tg model.Target) {
	g.mux.Lock()
	defer g.mux.Unlock()
	g.targets = append(g.targets, tg)
}

func newTarget(ip string, cred CredentialConfig, si snmputils.SysInfo) *target {
	tg := &target{
		IPAddress:  ip,
		Credential: cred,
		SysInfo:    si,
	}

	tg.hash, _ = hashstructure.Hash(tg, nil)

	return tg
}

type (
	target struct {
		model.Base `hash:"ignore"`
		hash       uint64

		IPAddress  string
		Credential CredentialConfig  `hash:"ignore"`
		SysInfo    snmputils.SysInfo `hash:"ignore"`
	}
)

func (t *target) TUID() string { return fmt.Sprintf("snmp_%s_%d", t.IPAddress, t.hash) }
func (t *target) Hash() uint64 { return t.hash }
