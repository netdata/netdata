//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRejectInvalidReleaseBeforeLaunch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Paths)
	}{
		{"contract", func(p *Paths) { p.Identity.Contract++ }},
		{"version", func(p *Paths) { p.Identity.Version = "" }},
		{"digest", func(p *Paths) { p.Identity.SHA256 = "not-a-digest" }},
		{"relative node", func(p *Paths) { p.Node = "node" }},
		{"relative entrypoint", func(p *Paths) { p.EntryPoint = "run.mjs" }},
		{"missing entrypoint", func(p *Paths) { p.EntryPoint += ".missing" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			source := e.source.(fixtureRuntime)
			tc.change(&source.Paths)
			e.source = source
			e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
				t.Fatal("invalid release launched")
				return nil, nil
			}
			require.Error(t, e.Check(context.Background(), synthetic.Journey))
			result := e.Execute(context.Background(), journey(), nil)
			require.Equal(t, synthetic.Error, result.Run.Outcome)
			require.Nil(t, result.Run.Runtime)
		})
	}
}
