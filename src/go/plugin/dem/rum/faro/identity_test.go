// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinnedSDKDocumentIdentity(t *testing.T) {
	requireNode(t)
	cmd := exec.Command("node", "testdata/identity-sdk.cjs")
	cmd.Stdin = strings.NewReader(Bootstrap("shop", BootstrapOptions{
		MeasureRate: 1,
	}))
	raw, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok {
		t.Log(string(exit.Stderr))
	}
	require.NoError(t, err)
	var bodies []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &bodies))
	var documents, views int
	for _, body := range bodies {
		b, err := Decode(body, Options{
			Now: now,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, b.ExperienceID)
		assert.Equal(t, "/entry", b.PageGroup)
		for _, event := range b.Events {
			switch event.Kind {
			case beacon.EventSession:
				assert.NotEmpty(t, event.ID)
				assert.Positive(t, event.Revision)
				assert.Equal(t, event.ID, event.Attrs["observation_id"])
			case beacon.EventDocument:
				documents++
				assert.Equal(t, b.ExperienceID, event.ID)
				assert.Positive(t, event.Revision)
			case beacon.EventView:
				views++
				assert.Equal(t, b.ViewID, event.ID)
				assert.Positive(t, event.Revision)
			}
		}
		for _, vital := range b.Vitals {
			assert.NotEmpty(t, vital.ID)
			assert.Positive(t, vital.Revision)
		}
		if b.Navigation != nil {
			assert.Positive(t, b.Navigation.Revision)
		}
		for _, resource := range b.Resources {
			assert.NotEmpty(t, resource.ID)
			assert.True(t, resource.HasDuration)
			assert.Zero(t, resource.DurationMS)
		}
	}
	assert.Equal(t, 2, documents)
	assert.Equal(t, 2, views)
}

func TestObservationIdentityNormalization(t *testing.T) {
	for _, tc := range []struct {
		name, id, revision     string
		validID, validRevision bool
	}{
		{"valid", "v5-1234567890123-1234567890123", "42", true, true},
		{"boundary", strings.Repeat("x", 128), "9007199254740991", true, true},
		{"missing", "", "", false, false},
		{"noncanonical revision", "metric", "01", true, false},
		{"unbounded revision", "metric", strings.Repeat("0", 1024) + "1", true, false},
		{"long", strings.Repeat("x", 129), "9007199254740992", false, false},
		{"control", "metric\n1", "-1", false, false},
		{"opaque URL", "https://example.org", "1.5", false, false},
		{"unicode", "metric☃", "0", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{
				"meta":         map[string]any{"session": map[string]string{"id": tc.id}, "page": map[string]string{"id": tc.id, "url": "https://example.org/entry"}, "view": map[string]string{"id": tc.id, "name": "checkout"}},
				"measurements": []any{map[string]any{"values": map[string]float64{"cls": 0}, "context": map[string]string{"id": tc.id, "observation_sequence": tc.revision}}},
				"events":       []any{map[string]any{"name": "document_activated", "attributes": map[string]string{"observation_id": tc.id, "observation_sequence": tc.revision}}},
			})
			require.NoError(t, err)
			b, err := Decode(raw, Options{
				Now: now,
			})
			require.NoError(t, err)
			require.Len(t, b.Vitals, 1)
			require.Len(t, b.Events, 1)
			assert.Equal(t, "/entry", b.PageGroup)
			assert.Equal(t, "checkout", b.View)
			assert.Equal(t, tc.validID, b.SessionID != "")
			assert.Equal(t, tc.validID, b.ExperienceID != "")
			assert.Equal(t, tc.validID, b.ViewID != "")
			assert.Equal(t, tc.validID, b.Vitals[0].ID != "")
			assert.Equal(t, tc.validID, b.Events[0].ID != "")
			assert.Equal(t, tc.validRevision, b.Vitals[0].Revision != 0)
			assert.Equal(t, tc.validRevision, b.Events[0].Revision != 0)
			assert.Equal(t, beacon.EventDocument, b.Events[0].Kind)
		})
	}
}

func TestDurationPresenceAndNavigationRevision(t *testing.T) {
	b, err := Decode([]byte(`{"events":[
  {"name":"faro.performance.navigation","attributes":{"pageLoadTime":"0","domContentLoadHandlerTime":"0","observation_sequence":"3"}},
  {"name":"faro.performance.navigation","attributes":{"pageLoadTime":"99","observation_sequence":"2"}},
  {"name":"faro.performance.resource","attributes":{"duration":"0","observation_id":"resource-1"}},
  {"name":"faro.performance.resource","attributes":{"httpHost":"example.org","observation_id":"resource-2"}},
  {"name":"session_start"},{"name":"session_resume"},{"name":"session_extend"}]}`), Options{
		Now: now,
	})
	require.NoError(t, err)
	assert.Equal(t, &beacon.Navigation{
		Revision: 3,
		HasLoad:  true,
		HasDCL:   true,
	}, b.Navigation)
	require.Len(t, b.Resources, 2)
	assert.True(t, b.Resources[0].HasDuration)
	assert.False(t, b.Resources[1].HasDuration)
	assert.Empty(t, b.Resources[0].Host, "unknown ownership stays distinct")
	for _, event := range b.Events[4:] {
		assert.Equal(t, beacon.EventSession, event.Kind)
	}
}

func TestPinnedSDKBotTransportURLExclusion(t *testing.T) {
	requireNode(t)
	cmd := exec.Command("node", "testdata/identity-sdk.cjs", "bot")
	cmd.Stdin = strings.NewReader(Bootstrap("shop", BootstrapOptions{
		MeasureRate: 1,
		IncludeBots: true,
	}))
	_, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok {
		t.Log(string(exit.Stderr))
	}
	require.NoError(t, err)
}
