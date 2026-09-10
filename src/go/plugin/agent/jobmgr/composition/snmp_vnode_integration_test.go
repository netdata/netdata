// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/apache"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	govnode "github.com/netdata/netdata/go/plugins/plugin/go.d/vnode"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// This peer accepts only its own community. The provider intentionally has no
// sysObjectID or metric data; the collector peer uses a stock metric profile.
type vnodeSNMPPeer struct {
	port     int
	enabled  atomic.Bool
	requests atomic.Int32
	metrics  atomic.Int32
}

func newVNodeSNMPPeer(t *testing.T, community, hostname string, metrics bool) *vnodeSNMPPeer {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(t, err)
	portNum, err := strconv.Atoi(port)
	require.NoError(t, err)
	peer := &vnodeSNMPPeer{port: portNum}
	peer.enabled.Store(true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			p, err := (&gosnmp.GoSNMP{}).SnmpDecodePacket(buf[:n])
			if err != nil || p.Community != community {
				continue
			}
			peer.requests.Add(1)
			if !peer.enabled.Load() {
				continue
			}
			r := &gosnmp.SnmpPacket{Version: p.Version, Community: p.Community, PDUType: gosnmp.GetResponse, RequestID: p.RequestID}
			for _, request := range p.Variables {
				v := gosnmp.SnmpPDU{Name: request.Name, Type: gosnmp.NoSuchObject}
				switch request.Name {
				case ".1.3.6.1.2.1.1.5.0":
					v.Type, v.Value = gosnmp.OctetString, hostname
				case ".1.3.6.1.2.1.1.1.0":
					v.Type, v.Value = gosnmp.OctetString, "synthetic device"
				default:
					if !strings.HasPrefix(request.Name, ".1.3.6.1.2.1.1.") {
						peer.metrics.Add(1)
						if metrics {
							v.Type, v.Value = gosnmp.Counter32, uint32(42)
						}
					}
				}
				if p.PDUType != gosnmp.GetRequest {
					v.Type, v.Value = gosnmp.EndOfMibView, nil
				}
				r.Variables = append(r.Variables, v)
			}
			raw, err := r.MarshalMsg()
			if err == nil {
				_, _ = conn.WriteTo(raw, addr)
			}
		}
	}()
	t.Cleanup(func() { _ = conn.Close(); <-done })
	return peer
}

func loadVNodeSNMPMetricProfile(t *testing.T) string {
	t.Helper()
	const name = "apc-ups"
	// Cold catalog loading is fixture setup, not part of the vnode readiness deadline.
	profiles := ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		ManualProfiles: []string{name},
	}).Project(ddsnmp.ConsumerMetrics).Profiles()
	require.Len(t, profiles, 1)
	require.NotEmpty(t, profiles[0].Definition.Metrics)
	return name
}

func TestSNMPVnodeFileAcquisitionAndNamedJobs(t *testing.T) {
	for _, withSNMPJob := range []bool{false, true} {
		t.Run(fmt.Sprint(withSNMPJob), func(t *testing.T) {
			provider := newVNodeSNMPPeer(t, "fixture-provider", "canonical-router", false)
			collector := newVNodeSNMPPeer(t, "fixture-collector", "collector-local-name", true)
			var apiRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				apiRequests.Add(1)
				_, _ = io.WriteString(w, "BusyWorkers: 1\nIdleWorkers: 4\nScoreboard: _W___\n")
			}))
			t.Cleanup(server.Close)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "vnodes.conf"), []byte(fmt.Sprintf(`
- name: router
  mode: snmp
  mode_snmp:
    address: 127.0.0.1
    port: %d
    timeout: 1m
    retries: 0
    credentials:
      community: fixture-provider
  labels:
    site: test
`, provider.port)), 0600))
			initial := vnodes.Load(dir, true)
			require.Len(t, initial, 1)
			var jobs []confgroup.Config
			jobYAML := fmt.Sprintf(`
- module: apache
  name: api
  url: %s
  vnode: router
  update_every: 1
  autodetection_retry: 1
`, server.URL+"?auto")
			if withSNMPJob {
				profile := loadVNodeSNMPMetricProfile(t)
				jobYAML += fmt.Sprintf(`
- module: snmp
  name: metrics
  hostname: 127.0.0.1
  community: fixture-collector
  options:
    port: %d
  vnode: router
  manual_profiles: [%s]
  update_every: 1
  autodetection_retry: 1
  ping:
    enabled: false
`, collector.port, profile)
			}
			require.NoError(t, yaml.Unmarshal([]byte(jobYAML), &jobs))
			for _, job := range jobs {
				job.SetSourceType(confgroup.TypeUser)
				job.SetSource("file=fixture")
				job.SetProvider("test")
			}
			store := ddsnmp.NewDeviceStore()
			reader, writer := io.Pipe()
			t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
			output := newProcessSynchronizedBuffer()
			config := testProductionProcessConfig(reader, output)
			config.InitialVnodes, config.SNMPVnodeAcquirer = initial, govnode.SNMP{}
			config.AutoEnable = true
			config.Modules = collectorapi.Registry{"apache": {Create: func() collectorapi.CollectorV1 { return apache.New() }, Config: func() any { return &apache.Config{} }}, "snmp": snmp.Creator(store, nil)}
			config.Defaults = confgroup.Registry{"apache": {}, "snmp": {}}
			config.DiscoveryProviders = []agentdiscovery.ProviderFactory{agentdiscovery.NewProviderFactory("test", func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
				return runTestDiscoverer{configs: jobs}, true, nil
			})}
			process, err := NewProcess(config)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() { done <- process.Run(ctx) }()
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("process did not join")
				}
			})
			guid := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("127.0.0.1")).String()
			require.Eventually(t, func() bool {
				return strings.Contains(output.String(), "HOST_DEFINE '"+guid+"' 'canonical-router'") && apiRequests.Load() > 0
			}, 8*time.Second, 10*time.Millisecond)
			require.NotContains(t, output.String(), "fixture-provider")
			require.Zero(t, provider.metrics.Load(), "acquisition must not read metric OIDs")
			if withSNMPJob {
				require.Eventually(t, func() bool { return len(store.Entries()) == 1 && collector.metrics.Load() > 0 }, 5*time.Second, 10*time.Millisecond)
				info := store.Entries()[0].Info
				require.Equal(t, "fixture-collector", info.Community)
				require.Equal(t, collector.port, info.Port)
				require.Equal(t, guid, info.VnodeGUID)
				require.Equal(t, "canonical-router", info.VnodeHostname)
				require.Equal(t, "test", info.VnodeLabels["site"])
			} else {
				require.Empty(t, store.Entries(), "provider must not register connections")
			}
			require.NotRegexp(t, `HOST_DEFINE '[^']*' 'collector-local-name'`, output.String(), "job-local names may appear in chart labels, never in host definitions")
			requestsBeforeEdit := provider.requests.Load()
			edited := initial["router"].Copy()
			edited.Hostname = "operator-router"
			edited.Labels["site"] = "updated"
			payload, err := json.Marshal(edited)
			require.NoError(t, err)
			_, err = fmt.Fprintf(writer, "FUNCTION_PAYLOAD vnode-edit 30 \"config go.d:vnode:router update\" 0xFFFF \"user=test\" application/json\n%s\nFUNCTION_PAYLOAD_END\n", payload)
			require.NoError(t, err)
			output.waitContains(t, "FUNCTION_RESULT_BEGIN vnode-edit 202 application/json")
			require.Eventually(t, func() bool {
				return strings.Contains(output.String(), "HOST_DEFINE '"+guid+"' 'operator-router'")
			}, 3*time.Second, 10*time.Millisecond)
			if withSNMPJob {
				require.Eventually(t, func() bool {
					entries := store.Entries()
					return len(entries) == 1 && entries[0].Info.VnodeHostname == "operator-router" && entries[0].Info.VnodeLabels["site"] == "updated"
				}, 3*time.Second, 10*time.Millisecond)
			}
			require.Equal(t, requestsBeforeEdit, provider.requests.Load(), "override-only edits must not acquire again")

			// Every run starts without acquired metadata. A long blocked UDP read
			// must be canceled and joined on both restart and termination.
			provider.enabled.Store(false)
			for range 2 {
				before := provider.requests.Load()
				restartCtx, restartCancel := context.WithTimeout(ctx, 3*time.Second)
				require.NoError(t, process.Restart(restartCtx))
				restartCancel()
				require.Eventually(t, func() bool { return provider.requests.Load() > before }, time.Second, time.Millisecond)
				afterRestart := len(output.String())
				require.Never(t, func() bool { return strings.Contains(output.String()[afterRestart:], "HOST_DEFINE") }, 100*time.Millisecond, 10*time.Millisecond)
			}
			require.NoError(t, process.Terminate(ctx))
		})
	}
}

func TestSNMPLegacyLocalVnodeDynCfgRoundTrip(t *testing.T) {
	profile := loadVNodeSNMPMetricProfile(t)
	peer := newVNodeSNMPPeer(t, "fixture-local", "device-name", true)
	var jobs []confgroup.Config
	require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf(`
- module: snmp
  name: local
  hostname: 127.0.0.1
  community: fixture-local
  options:
    port: %d
  vnode:
    guid: 6e17cd2c-0518-4e94-965b-d25673decb21
    hostname: local-router
    labels:
      site: before
  manual_profiles: [%s]
  update_every: 1
  ping:
    enabled: false
`, peer.port, profile)), &jobs))
	jobs[0].SetSourceType(confgroup.TypeUser)
	jobs[0].SetSource("file=legacy-fixture")
	jobs[0].SetProvider("test")
	store := ddsnmp.NewDeviceStore()
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	output := newProcessSynchronizedBuffer()
	config := testProductionProcessConfig(reader, output)
	config.AutoEnable = true
	config.Modules = collectorapi.Registry{"snmp": snmp.Creator(store, nil)}
	config.Defaults = confgroup.Registry{"snmp": {}}
	config.DiscoveryProviders = []agentdiscovery.ProviderFactory{agentdiscovery.NewProviderFactory("test", func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
		return runTestDiscoverer{configs: jobs}, true, nil
	})}
	process, err := NewProcess(config)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("process did not join")
		}
	})
	require.Eventually(t, func() bool {
		return strings.Contains(output.String(), "HOST_DEFINE '6e17cd2c-0518-4e94-965b-d25673decb21' 'local-router'")
	}, 5*time.Second, 10*time.Millisecond)
	_, err = io.WriteString(writer, "FUNCTION legacy-get 30 \"config go.d:collector:snmp:local get\" 0xFFFF \"user=test\"\n")
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN legacy-get 200 application/json")
	wire := output.String()
	start := strings.Index(wire, "FUNCTION_RESULT_BEGIN legacy-get ")
	result := wire[start:]
	_, result, _ = strings.Cut(result, "\n")
	result, _, _ = strings.Cut(result, "\nFUNCTION_RESULT_END")
	var editable map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &editable))
	require.Equal(t, "", editable["vnode"])
	local := editable["local_vnode"].(map[string]any)
	require.Equal(t, "local-router", local["hostname"])
	require.Equal(t, "6e17cd2c-0518-4e94-965b-d25673decb21", local["guid"])
	require.Equal(t, "before", local["labels"].(map[string]any)["site"])
	local["labels"].(map[string]any)["site"] = "after"
	payload, err := json.Marshal(editable)
	require.NoError(t, err)
	_, err = fmt.Fprintf(writer, "FUNCTION_PAYLOAD local-save 30 \"config go.d:collector:snmp:local update\" 0xFFFF \"user=test\" application/json\n%s\nFUNCTION_PAYLOAD_END\n", payload)
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN local-save 200 application/json")
	require.Eventually(t, func() bool {
		entries := store.Entries()
		return len(entries) == 1 && entries[0].Info.VnodeHostname == "local-router" && entries[0].Info.VnodeGUID == "6e17cd2c-0518-4e94-965b-d25673decb21" && entries[0].Info.VnodeLabels["site"] == "after"
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, process.Terminate(ctx))
}
