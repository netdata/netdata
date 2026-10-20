// SPDX-License-Identifier: GPL-3.0-or-later

package hostoutput

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func definition(t *testing.T, guid, hostname string, labels map[string]string) *Definition {
	t.Helper()
	d, err := NewDefinition(netdataapi.HostInfo{
		GUID:     guid,
		Hostname: hostname,
		Labels:   labels,
	})
	require.NoError(t, err)
	return d
}

func emit(t *testing.T, owner *Owner, d *Definition, cleanup bool) (string, *Publication) {
	t.Helper()
	tx := Prepare(
		Request{
			Owner:      owner,
			Definition: d,
			Payload:    []byte("HOST '" + owner.guid + "'\n"),
			Cleanup:    cleanup,
		},
		nil,
	)
	wire, err := tx.Build()
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	return string(wire), tx
}

func TestAuthorityTransitions(t *testing.T) {
	for name, tc := range map[string]struct{ removeBeforeResume, readd bool }{
		"quiet throughout adoption and removal": {removeBeforeResume: true},
		"quiet adoption":                        {},
		"remove and readd":                      {removeBeforeResume: true, readd: true},
	} {
		t.Run(name, func(t *testing.T) {
			p := New()
			a := p.NewOwner("node")
			b := p.NewOwner("node")
			fallback := definition(t, "node", "generated", map[string]string{"vendor": "cisco"})
			configured := definition(t, "node", "configured", map[string]string{"site": "athens"})
			wire, _ := emit(t, a, fallback, false)
			require.Contains(t, wire, "HOST_DEFINE 'node' 'generated'")
			p.Bind(func(string) *Definition { return configured })
			wire, _ = emit(t, b, fallback, false)
			require.Contains(t, wire, "HOST_DEFINE 'node' 'configured'")
			b.Release()
			if tc.removeBeforeResume {
				p.Bind(nil)
			}
			if tc.readd {
				configured = definition(t, "node", "readded", nil)
				p.Bind(func(string) *Definition { return configured })
			}
			wire, _ = emit(t, a, fallback, false)
			switch {
			case tc.readd:
				assert.Contains(t, wire, "HOST_DEFINE 'node' 'readded'")
			case tc.removeBeforeResume:
				assert.Contains(t, wire, "HOST_DEFINE 'node' 'generated'")
			default:
				assert.NotContains(t, wire, "HOST_DEFINE")
			}
			wire, _ = emit(t, a, fallback, false)
			assert.NotContains(t, wire, "HOST_DEFINE")
			a.Release()
			assert.Zero(t, p.Len())
		})
	}
}

func TestCleanupAuthority(t *testing.T) {
	for name, tc := range map[string]struct {
		fallback, configured map[string]string
		want                 string
	}{
		"configured timeout suppresses cleanup":           {configured: map[string]string{"_node_stale_after_seconds": "60"}, want: "global"},
		"configured disabled overrides generated timeout": {fallback: map[string]string{"_node_stale_after_seconds": "60"}, configured: map[string]string{}, want: "hostglobal"},
		"generated timeout":                               {fallback: map[string]string{"_node_stale_after_seconds": "60"}, want: "global"},
	} {
		t.Run(name, func(t *testing.T) {
			p := New()
			o := p.NewOwner("node")
			defer o.Release()
			d := definition(t, "node", "fallback", tc.fallback)
			emit(t, o, d, false)
			if tc.configured != nil {
				configured := definition(t, "node", "configured", tc.configured)
				p.Bind(func(string) *Definition { return configured })
			}
			tx := Prepare(
				Request{
					Owner:      o,
					Definition: d,
					Payload:    []byte("host"),
					Tail:       []byte("global"),
					Cleanup:    true,
				},
				nil,
			)
			wire, err := tx.Build()
			require.NoError(t, err)
			require.NoError(t, tx.Commit())
			assert.Equal(t, tc.want, string(wire))
		})
	}
}

func TestAbortedPublication(t *testing.T) {
	for name, tc := range map[string]struct{ initial bool }{"first publication": {}, "existing quiet owner": {initial: true}} {
		t.Run(name, func(t *testing.T) {
			p := New()
			o := p.NewOwner("node")
			defer o.Release()
			d := definition(t, "node", "generated", nil)
			if tc.initial {
				emit(t, o, d, false)
			}
			configured := definition(t, "node", "configured", nil)
			p.Bind(func(string) *Definition { return configured })
			tx := Prepare(Request{
				Owner:      o,
				Definition: d,
				Payload:    []byte("samples"),
			}, nil)
			_, err := tx.Build()
			require.NoError(t, err)
			require.NoError(t, tx.Abort())
			wire, _ := emit(t, o, d, false)
			assert.Contains(t, wire, "HOST_DEFINE 'node' 'configured'")
		})
	}
}

func TestGeneratedConflictsRemainStableAndBounded(t *testing.T) {
	p := New()
	a := p.NewOwner("node")
	b := p.NewOwner("node")
	da := definition(t, "node", "a", nil)
	db := definition(t, "node", "b", nil)
	emit(t, a, da, false)
	_, tx := emit(t, b, db, false)
	require.NotNil(t, tx.Conflict)
	for range 3 {
		for _, v := range []struct {
			o *Owner
			d *Definition
		}{{a, da}, {b, db}} {
			wire, tx := emit(t, v.o, v.d, false)
			assert.NotContains(t, wire, "HOST_DEFINE")
			assert.Nil(t, tx.Conflict)
		}
	}
	for i := range 100 {
		emit(t, b, definition(t, "node", fmt.Sprintf("b-%d", i), nil), false)
	}
	assert.Len(t, p.hosts["node"].reported, 64)
	assert.Len(t, p.hosts["node"].reportOrder, 64)
	a.Release()
	b.Release()
	assert.Zero(t, p.Len())
}

func TestGUIDAliasesShareAuthority(t *testing.T) {
	const guid = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	for name, tc := range map[string]struct{ alias string }{
		"canonical":         {alias: guid},
		"uppercase compact": {alias: "AAAAAAAABBBBCCCCDDDDEEEEEEEEEEEE"},
	} {
		t.Run(name, func(t *testing.T) {
			p := New()
			configured := definition(t, guid, "configured", nil)
			p.Bind(func(key string) *Definition {
				require.Equal(t, guid, key)
				return configured
			})
			owner := p.NewOwner(tc.alias)
			d := definition(t, tc.alias, "generated", nil)
			wire, _ := emit(t, owner, d, false)
			assert.Contains(t, wire, "HOST_DEFINE '"+guid+"' 'configured'")
			owner.Release()
		})
	}
}
