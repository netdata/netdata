// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/require"
)

func TestPinnedSDKStartupReplayDoesNotRefreshSessions(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required for the shipped SDK fixture")
	}
	cmd := exec.Command("node", "../faro/testdata/identity-sdk.cjs")
	cmd.Stdin = strings.NewReader(faro.Bootstrap("shop", faro.BootstrapOptions{
		MeasureRate: 1,
	}))
	raw, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok {
		t.Log(string(exit.Stderr))
	}
	require.NoError(t, err)
	var bodies []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &bodies))
	require.NotEmpty(t, bodies)
	now := time.Now()
	b, err := faro.Decode(bodies[0], faro.Options{
		Site: "shop",
		Now:  now.Add(-2 * time.Minute),
	})
	require.NoError(t, err)
	var lifecycle []beacon.Event
	for _, event := range b.Events {
		if event.Kind == beacon.EventSession {
			lifecycle = append(lifecycle, event)
		}
	}
	require.NotEmpty(t, lifecycle, "exercise the actual startup envelope")
	a := aggregate.New(time.Minute, aggregate.SiteCfg{
		Name: "shop",
		Investigate: aggregate.InvestigateCfg{
			Rate: 1,
		},
	})
	a.Ingest(b)
	require.Zero(t, a.Activity().ObservedSessions)
	// Decode the identical serialized envelope again, with a later receipt time.
	replay, err := faro.Decode(bodies[0], faro.Options{
		Site: "shop",
		Now:  now,
	})
	require.NoError(t, err)
	result := a.Ingest(replay)
	require.Zero(t, a.Activity().ObservedSessions, "transport replay is not a fresh session observation")
	require.Empty(t, result.Observation.Events)
	require.Empty(t, result.Observation.Vitals)
	for _, event := range lifecycle {
		require.NotEmpty(t, event.ID)
		require.Positive(t, event.Revision)
	}
}
