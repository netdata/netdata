// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Keep one planner across real collection cycles: a fresh planner cannot detect accidental retirement.
func TestCollector_IPSecChartsSurviveUnavailableTelemetry(t *testing.T) {
	for name, tc := range map[string]struct {
		command string
		context string
		body    []byte
	}{
		"flow API failure":    {command: ipsecFlowCommand, context: "panos.ipsec.tunnel.state"},
		"gateway API failure": {command: ipsecGatewayCommand, context: "panos.ipsec.gateway.state"},
		"SA API failure":      {command: ipsecIKESACommand, context: "panos.ipsec.gateway.state"},
		"SA incomplete timestamps": {
			command: ipsecIKESACommand,
			context: "panos.ipsec.gateway.state",
			body:    []byte(strings.Replace(string(dataIPSecIKESA), "<created>Jan.01 00:00:00</created>", "", 1)),
		},
		"flow missing state": {
			command: ipsecFlowCommand,
			context: "panos.ipsec.tunnel.state",
			body:    []byte(strings.ReplaceAll(string(dataIPSecFlow), "<state>active</state>", "<state/>")),
		},
		"flow unidentified entry": {
			command: ipsecFlowCommand,
			context: "panos.ipsec.tunnel.state",
			body:    []byte(`<response status="success"><result><IPSec><entry><id>1</id></entry></IPSec></result></response>`),
		},
		"gateway unidentified entry": {
			command: ipsecGatewayCommand,
			context: "panos.ipsec.gateway.state",
			body:    []byte(`<response status="success"><result><entries><entry><id>1</id></entry></entries></result></response>`),
		},
		"flow unreadable list": {
			command: ipsecFlowCommand,
			context: "panos.ipsec.tunnel.state",
			body:    []byte(`<response status="success"><result><IPSec><error>unavailable</error></IPSec></result></response>`),
		},
		"gateway unreadable list": {
			command: ipsecGatewayCommand,
			context: "panos.ipsec.gateway.state",
			body:    []byte(`<response status="success"><result><entries>unavailable</entries></result></response>`),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Logger = logger.NewWithWriter(io.Discard)
			api := &mockAPIClient{}
			c.apiClient = api
			engine := newIPSecChartEngine(t, c)
			initial := collectIPSecChartPlan(t, c, engine)
			targets := ipsecCreatedCharts(initial, tc.context)
			require.NotEmpty(t, targets)
			if tc.body != nil {
				api.responses = map[string][]byte{tc.command: tc.body}
			} else {
				api.errors = map[string]error{tc.command: errors.New("telemetry unavailable")}
			}
			for cycle := 0; cycle < 12; cycle++ {
				actions := collectIPSecChartPlan(t, c, engine)
				assertIPSecChartObservations(t, actions, targets, true)
			}
			api.responses, api.errors = nil, nil
			assertIPSecChartObservations(t, collectIPSecChartPlan(t, c, engine), targets, false)
		})
	}
}

func TestCollector_IPSecConfirmedMembership(t *testing.T) {
	for name, tc := range map[string]struct {
		context   string
		responses map[string][]byte
		errors    map[string]error
		removed   int
		retained  int
		gaps      bool
	}{
		"empty SA list keeps gateways measured down": {
			context:   "panos.ipsec.gateway.state",
			responses: map[string][]byte{ipsecIKESACommand: dataIPSecIKESAEmpty},
			retained:  4,
		},
		"empty gateway inventory retires all gateways": {
			context:   "panos.ipsec.gateway.state",
			responses: map[string][]byte{ipsecGatewayCommand: []byte(`<response status="success"><result><entries/></result></response>`)},
			removed:   4,
		},
		"empty flow inventory retires all tunnels": {
			context:   "panos.ipsec.tunnel.state",
			responses: map[string][]byte{ipsecFlowCommand: []byte(`<response status="success"><result><IPSec/></result></response>`)},
			removed:   6,
		},
		"gateway removals still apply when SA telemetry fails": {
			context: "panos.ipsec.gateway.state",
			responses: map[string][]byte{ipsecGatewayCommand: []byte(`<response status="success"><result><entries>` +
				`<entry><id>3</id><name>gw-branch-c</name><v2/></entry>` +
				`<entry><id>4</id><name>gw-branch-d</name><v2/></entry></entries></result></response>`)},
			errors:   map[string]error{ipsecIKESACommand: errors.New("telemetry unavailable")},
			removed:  2,
			retained: 2,
			gaps:     true,
		},
		"tunnel removals still apply when remaining state is unavailable": {
			context: "panos.ipsec.tunnel.state",
			responses: map[string][]byte{ipsecFlowCommand: []byte(`<response status="success"><result><IPSec>` +
				`<entry><name>branch-a</name><id>1</id></entry></IPSec></result></response>`)},
			removed:  5,
			retained: 1,
			gaps:     true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Logger = logger.NewWithWriter(io.Discard)
			api := &mockAPIClient{}
			c.apiClient = api
			engine := newIPSecChartEngine(t, c)
			targets := ipsecCreatedCharts(collectIPSecChartPlan(t, c, engine), tc.context)
			api.responses, api.errors = tc.responses, tc.errors
			actions := collectIPSecChartPlan(t, c, engine)
			removed := map[string]bool{}
			for _, action := range actions {
				if a, ok := action.(chartengine.RemoveChartAction); ok && targets[a.ChartID] {
					removed[a.ChartID] = true
				}
			}
			assert.Len(t, removed, tc.removed, "confirmed removals must retire in this successful cycle")
			for id := range removed {
				delete(targets, id)
			}
			require.Len(t, targets, tc.retained)
			assertIPSecChartObservations(t, actions, targets, tc.gaps)
			if tc.removed == 0 {
				value, ok := c.MetricStore().Read(metrix.ReadFlatten()).Value("ipsec_gateway_state_down", metrix.Labels{
					"gateway": "gw-branch-a", "ike_version": "ikev1", "measure_field": "down",
				})
				require.True(t, ok)
				assert.Equal(t, metrix.SampleValue(1), value)
			}
		})
	}
}

func TestCollector_IPSecInventoryBounds(t *testing.T) {
	c := New()
	c.Logger = logger.NewWithWriter(io.Discard)
	api := &mockAPIClient{}
	c.apiClient = api
	engine := newIPSecChartEngine(t, c)
	collectIPSecChartPlan(t, c, engine)
	for i := 0; i < 20; i++ {
		api.responses = map[string][]byte{
			ipsecGatewayCommand: []byte(fmt.Sprintf(`<response status="success"><result><entries>`+
				`<entry><name>gateway-%d</name><v2/></entry></entries></result></response>`, i)),
			ipsecFlowCommand: []byte(fmt.Sprintf(`<response status="success"><result><IPSec>`+
				`<entry><name>tunnel-%d</name><id>1</id><state>active</state></entry></IPSec></result></response>`, i)),
		}
		collectIPSecChartPlan(t, c, engine)
		assert.Len(t, c.ipsecGatewayInventory, 1)
		assert.Len(t, c.ipsecTunnelInventory, 1)
		// Invalid snapshots cannot replace known membership or accumulate each newly named row.
		api.responses[ipsecGatewayCommand] = []byte(fmt.Sprintf(`<response status="success"><result><entries>`+
			`<entry><name>unconfirmed-%d</name></entry><entry/></entries></result></response>`, i))
		api.responses[ipsecFlowCommand] = []byte(fmt.Sprintf(`<response status="success"><result><IPSec>`+
			`<entry><name>unconfirmed-%d</name><state>active</state></entry><entry/></IPSec></result></response>`, i))
		collectIPSecChartPlan(t, c, engine)
		assert.Len(t, c.ipsecGatewayInventory, 1)
		assert.Len(t, c.ipsecTunnelInventory, 1)
		assert.Contains(t, c.ipsecGatewayInventory, [2]string{fmt.Sprintf("gateway-%d", i), "ikev2"})
		assert.Contains(t, c.ipsecTunnelInventory, [2]string{fmt.Sprintf("tunnel-%d", i), "1"})
	}
}

func TestCollector_IPSecLifecycleProductionAdapter(t *testing.T) {
	var failSA atomic.Bool
	responses := map[string][]byte{
		systemInfoCommand:   dataSystemInfo,
		haStateCommand:      dataHAState,
		environmentCommand:  dataEnvironment,
		licenseInfoCommand:  dataLicenses,
		ipsecSACommand:      dataIPSecSA,
		ipsecFlowCommand:    dataIPSecFlow,
		ipsecGatewayCommand: dataIPSecGateway,
		ipsecIKESACommand:   dataIPSecIKESA,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		command := r.Form.Get("cmd")
		w.Header().Set("Content-Type", "application/xml")
		if command == ipsecIKESACommand && failSA.Load() {
			_, _ = io.WriteString(w, `<response status="error" code="2"><msg><line>internal error</line></msg></response>`)
			return
		}
		body := responses[command]
		if body == nil {
			body = []byte(`<response status="success"><result/></response>`)
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	c := New()
	c.Logger = logger.NewWithWriter(io.Discard)
	c.URL, c.APIKey = server.URL, "synthetic-test-key"
	require.NoError(t, c.Init(context.Background()))
	defer c.Cleanup(context.Background())
	require.IsType(t, &pangoAPIClient{}, c.apiClient)
	engine := newIPSecChartEngine(t, c)
	targets := ipsecCreatedCharts(collectIPSecChartPlan(t, c, engine), "panos.ipsec.gateway.state")
	require.Len(t, targets, 4)
	failSA.Store(true)
	for cycle := 0; cycle < 12; cycle++ {
		assertIPSecChartObservations(t, collectIPSecChartPlan(t, c, engine), targets, true)
	}
	failSA.Store(false)
	assertIPSecChartObservations(t, collectIPSecChartPlan(t, c, engine), targets, false)
}

func TestCollector_IPSecInitialInventoryFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		command   string
		context   string
		initial   int
		recovered int
	}{
		"unknown flow membership":             {command: ipsecFlowCommand, context: "panos.ipsec.tunnel.state", recovered: 6},
		"unknown gateway membership":          {command: ipsecGatewayCommand, context: "panos.ipsec.gateway.state", recovered: 4},
		"known gateways without SA telemetry": {command: ipsecIKESACommand, context: "panos.ipsec.gateway.state", initial: 4, recovered: 4},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Logger = logger.NewWithWriter(io.Discard)
			api := &mockAPIClient{errors: map[string]error{tc.command: errors.New("unavailable")}}
			c.apiClient = api
			engine := newIPSecChartEngine(t, c)
			actions := collectIPSecChartPlan(t, c, engine)
			targets := ipsecCreatedCharts(actions, tc.context)
			require.Len(t, targets, tc.initial)
			for _, action := range actions {
				if a, ok := action.(chartengine.UpdateChartAction); ok && targets[a.ChartID] {
					for _, value := range a.Values {
						assert.True(t, value.IsEmpty)
					}
				}
			}
			api.errors = nil
			actions = collectIPSecChartPlan(t, c, engine)
			if tc.initial == 0 {
				assert.Len(t, ipsecCreatedCharts(actions, tc.context), tc.recovered)
			} else {
				assertIPSecChartObservations(t, actions, targets, false)
			}
		})
	}
}

func newIPSecChartEngine(t *testing.T, c *Collector) *chartengine.Engine {
	t.Helper()
	engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), 1))
	return engine
}

func collectIPSecChartPlan(t *testing.T, c *Collector, engine *chartengine.Engine) []chartengine.EngineAction {
	t.Helper()
	mx, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	require.Contains(t, mx, metricKey("system_uptime", systemLabels()))
	attempt, err := engine.PreparePlan(c.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	actions := append([]chartengine.EngineAction(nil), attempt.Plan().Actions...)
	require.NoError(t, attempt.Commit())
	return actions
}

func ipsecCreatedCharts(actions []chartengine.EngineAction, context string) map[string]bool {
	charts := map[string]bool{}
	for _, action := range actions {
		if a, ok := action.(chartengine.CreateChartAction); ok && a.Meta.Context == context {
			charts[a.ChartID] = true
		}
	}
	return charts
}

func assertIPSecChartObservations(t *testing.T, actions []chartengine.EngineAction, targets map[string]bool, unavailable bool) {
	t.Helper()
	updates := map[string]bool{}
	for _, action := range actions {
		switch a := action.(type) {
		case chartengine.CreateChartAction:
			assert.False(t, targets[a.ChartID], "retained chart was recreated: %s", a.ChartID)
		case chartengine.RemoveChartAction:
			assert.False(t, targets[a.ChartID], "configured chart was removed: %s", a.ChartID)
		case chartengine.UpdateChartAction:
			if !targets[a.ChartID] {
				continue
			}
			updates[a.ChartID] = true
			ones := 0
			for _, value := range a.Values {
				assert.Equal(t, unavailable, value.IsEmpty, "%s/%s", a.ChartID, value.Name)
				if value.Int64 == 1 {
					ones++
				}
			}
			if !unavailable {
				assert.Equal(t, 1, ones, "one state must be active in %s", a.ChartID)
			}
		}
	}
	assert.Equal(t, targets, updates, "every retained chart must update, including empty measurements")
}
