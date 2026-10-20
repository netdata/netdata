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

type lifecycleHistory []aggregate.HistoryEvent

func (h *lifecycleHistory) Event(event aggregate.HistoryEvent) { *h = append(*h, event) }

func TestSessionLifecycleOccurrenceAdmission(t *testing.T) {
	for _, name := range []string{"session_start", "session_resume", "session_extend"} {
		t.Run(name, func(t *testing.T) {
			now := time.Now()
			decode := func(id, revision string, received time.Time) *beacon.Beacon {
				raw, err := json.Marshal(map[string]any{
					"meta":   map[string]any{"page": map[string]string{"id": "document", "url": "https://example.org/entry"}, "session": map[string]string{"id": "session"}},
					"events": []any{map[string]any{"name": name, "attributes": map[string]string{"observation_id": id, "observation_sequence": revision}}},
				})
				require.NoError(t, err)
				b, err := faro.Decode(raw, faro.Options{
					Site: "shop",
					Now:  received,
				})
				require.NoError(t, err)
				return b
			}
			a := aggregate.New(time.Minute, aggregate.SiteCfg{
				Name: "shop",
				Investigate: aggregate.InvestigateCfg{
					Rate: 1,
				},
			})
			var history lifecycleHistory
			a.SetHistorySink(&history)
			first := a.Ingest(decode("occurrence-1", "1", now.Add(-2*time.Minute)))
			require.Len(t, first.Observation.Events, 1)
			require.Zero(t, a.Activity().ObservedSessions)
			// An occurrence is counted once, including reports with a higher revision.
			for _, revision := range []string{"1", "2"} {
				replay := a.Ingest(decode("occurrence-1", revision, now))
				require.Empty(t, replay.Observation.Events)
				require.Zero(t, a.Activity().ObservedSessions)
			}
			fresh := a.Ingest(decode("occurrence-2", "3", now))
			require.Len(t, fresh.Observation.Events, 1)
			require.EqualValues(t, 1, a.Activity().ObservedSessions)
			require.Zero(t, a.Activity().PageviewsWindow)
			require.Zero(t, a.Snapshot().Counters[aggregate.CounterInvalidMeasurements])
			require.Empty(t, history, "session lifecycle does not produce generic history rows")
			events, found := a.SessionEvents("session")
			require.True(t, found)
			require.Empty(t, events, "session lifecycle does not produce native timeline rows")
		})
	}
}

func TestSessionLifecycleRequiresIdentity(t *testing.T) {
	for _, missing := range []string{"experience", "session", "id", "revision"} {
		t.Run(missing, func(t *testing.T) {
			b := &beacon.Beacon{
				Site:         "shop",
				Received:     time.Now(),
				ExperienceID: "document",
				SessionID:    "session",
				Events:       []beacon.Event{{Kind: beacon.EventSession, Name: "session_start", ID: "occurrence", Revision: 1}},
			}
			switch missing {
			case "experience":
				b.ExperienceID = ""
			case "session":
				b.SessionID = ""
			case "id":
				b.Events[0].ID = ""
			case "revision":
				b.Events[0].Revision = 0
			}
			a := aggregate.New(time.Minute, aggregate.SiteCfg{
				Name: "shop",
			})
			require.Empty(t, a.Ingest(b).Observation.Events)
			require.Zero(t, a.Activity().ObservedSessions)
			require.EqualValues(t, 1, a.Snapshot().Counters[aggregate.CounterInvalidMeasurements])
		})
	}
}
