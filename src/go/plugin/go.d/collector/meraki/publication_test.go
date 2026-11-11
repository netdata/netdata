// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *lockedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }

// startManagedJob starts c as a real managed JobV2 that publishes into out.
func startManagedJob(t *testing.T, c *Collector, out *lockedBuffer, publisher *hostoutput.Publisher) *jobruntime.JobV2 {
	t.Helper()
	job := jobruntime.NewJobV2(
		jobruntime.JobV2Config{
			PluginName:  "go.d",
			Name:        "fixture",
			ModuleName:  "meraki",
			FullName:    "meraki_fixture",
			Module:      c,
			Out:         out,
			UpdateEvery: 1,
			Publication: publisher,
		},
	)
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	run := jobruntime.NewManagedRun(t.Context(), nil)
	done := make(chan struct{})
	go func() { defer close(done); job.StartManaged(run) }()
	t.Cleanup(func() {
		run.Stop(nil)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("managed job did not stop")
		}
		c.Cleanup(context.Background())
	})
	select {
	case <-run.StartupDone():
	case <-time.After(5 * time.Second):
		t.Fatal("managed startup blocked")
	}
	require.NoError(t, run.StartupErr())
	return job
}

func TestManagedDevicePublicationAndRetirement(t *testing.T) {
	f, server := newFixtureAPI(t)
	f.set("/organizations/org/devices/availabilities", fixtureResponse{
		body: `[]`,
	})
	c := configuredCollector(server)
	out := &lockedBuffer{}
	publisher := hostoutput.New()
	job := startManagedJob(t, c, out, publisher)
	statusBegin := regexp.MustCompile(`(?m)^BEGIN '[^']+_data_collection_status'`)
	tick := func() string {
		t.Helper()
		before := out.String()
		count := len(statusBegin.FindAllStringIndex(before, -1))
		require.Eventually(t, func() bool {
			if len(statusBegin.FindAllStringIndex(out.String(), -1)) > count {
				return true
			}
			job.Tick(0)
			return false
		}, 4*time.Second, 120*time.Millisecond)
		return strings.TrimPrefix(out.String(), before)
	}
	wire := tick()
	device := readDevices(c)["S1"]
	guid := device.Scope.GUID
	require.NotEmpty(t, guid)
	assert.Contains(t, wire, "HOST_DEFINE '"+guid+"' 'Access Point'")
	assert.Contains(t, wire, "BEGIN 'meraki_fixture.meraki_device_assigned'")
	assert.NotContains(t, wire, "BEGIN 'meraki_fixture.meraki_device_availability'")
	require.Equal(t, 1, publisher.OwnerCount(guid))

	f.set("/organizations/org/devices/availabilities", fixtureResponse{
		body: `[{"serial":"S1","status":"online"}]`,
	})
	wire = tick()
	assert.Contains(t, wire, "BEGIN 'meraki_fixture.meraki_device_availability'")
	assert.Equal(t, wantStatus("online"), readDevices(c)["S1"].Values)
	f.set("/organizations/org/devices/availabilities", fixtureResponse{
		body: `[]`,
	})
	wire = tick()
	assert.NotContains(t, wire, "BEGIN 'meraki_fixture.meraki_device_availability'")
	require.Equal(t, 1, publisher.OwnerCount(guid))

	f.set("/organizations/org/devices", fixtureResponse{
		body: `[]`,
	})
	for range 3 {
		tick()
	}
	require.Equal(t, 1, publisher.OwnerCount(guid))
	f.set("/organizations/org/devices", fixtureResponse{
		status: 500,
		body:   `{}`,
	})
	for range 3 {
		wire = tick()
		assert.NotContains(t, wire, "BEGIN 'meraki_fixture.meraki_device_assigned'")
		require.Equal(t, 1, publisher.OwnerCount(guid), "failures must not advance scope retirement")
	}
	f.set("/organizations/org/devices", fixtureResponse{
		body: `[]`,
	})
	for range 3 {
		tick()
	}
	assert.Equal(t, 0, publisher.OwnerCount(guid))
	assert.Contains(t, out.String(), "obsolete")
}

func TestManagedTrafficPlacementAndFloatValues(t *testing.T) {
	_, c, _ := newTrafficFixture(t)
	out := &lockedBuffer{}
	job := startManagedJob(t, c, out, hostoutput.New())
	require.Eventually(t, func() bool {
		if strings.Contains(out.String(), "meraki.appliance_uplink_traffic") {
			return true
		}
		job.Tick(0)
		return false
	}, 5*time.Second, 120*time.Millisecond)

	// Map each traffic context to the HOST its chart was defined under.
	wire := out.String()
	hostLine := regexp.MustCompile(`^HOST '([^']*)'`)
	chartLine := regexp.MustCompile(`^CHART .* '(meraki\.(switch_port|appliance_uplink)[a-z_]*)' `)
	host := ""
	hosts := make(map[string]string)
	for _, line := range strings.Split(wire, "\n") {
		if m := hostLine.FindStringSubmatch(line); m != nil {
			host = m[1]
		}
		if m := chartLine.FindStringSubmatch(line); m != nil {
			hosts[m[1]] = host
		}
	}
	guid := readDevices(c)["S1"].Scope.GUID
	require.NotEmpty(t, guid)
	assert.Equal(t, map[string]string{
		"meraki.switch_port_traffic":              guid,
		"meraki.switch_port_traffic_interval_age": guid,
		"meraki.switch_port_packets":              guid,
		"meraki.appliance_uplink_traffic":         "",
	}, hosts)
	for _, value := range []string{"10.5", "20.25", "2.5", "3.25"} {
		assert.Regexp(t, regexp.MustCompile(`(?m)^SET '[^']+' = `+regexp.QuoteMeta(value)+`$`), wire)
	}
}
