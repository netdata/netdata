// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
)

type targetGroup struct {
	source  string
	targets []model.Target
}

func (g *targetGroup) Provider() string        { return fullName }
func (g *targetGroup) Source() string          { return g.source }
func (g *targetGroup) Targets() []model.Target { return g.targets }

type target struct {
	model.Base `hash:"ignore"`

	hash  uint64
	label string

	Item any
}

func (t *target) TUID() string { return fmt.Sprintf("http_%s_%x", t.label, t.hash) }
func (t *target) Hash() uint64 { return t.hash }
