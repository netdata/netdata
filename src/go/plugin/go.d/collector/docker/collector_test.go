// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	typesContainer "github.com/moby/moby/api/types/container"
	typesImage "github.com/moby/moby/api/types/image"
	typesSystem "github.com/moby/moby/api/types/system"
	docker "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestArtifacts(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)

	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	health, err := os.ReadFile("../../../../../health/health.d/docker.conf")
	require.NoError(t, err)

	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, templateYAML, nil)
	collecttest.AssertHealthAlertsTargetChartTemplate(t, health, templateYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default config": {
			wantFail: false,
			config:   New().Config,
		},
		"unset 'address'": {
			wantFail: false,
			config: Config{
				Address: "",
			},
		},
		"bad 'container_selector'": {
			wantFail: true,
			config: Config{
				Address:           docker.DefaultDockerHost,
				ContainerSelector: "[",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func(*Collector)
		wantClose bool
	}{
		"after New": {
			wantClose: false,
			prepare:   func(*Collector) {},
		},
		"after Init": {
			wantClose: false,
			prepare:   func(c *Collector) { _ = c.Init(context.Background()) },
		},
		"after Check": {
			wantClose: true,
			prepare:   func(c *Collector) { _ = c.Init(context.Background()); _ = c.Check(context.Background()) },
		},
		"after Collect": {
			wantClose: true,
			prepare: func(c *Collector) {
				_ = c.Init(context.Background())
				_, _ = collecttest.CollectScalarSeries(c)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := &mockClient{}
			collr := New()
			collr.newClient = prepareNewClientFunc(m)

			test.prepare(collr)

			require.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			assert.Equal(t, test.wantClose, m.closeCalled)

			m.closeCalled = false
			require.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			assert.False(t, m.closeCalled, "second Cleanup must not close the client again")
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		wantFail bool
	}{
		"success": {
			wantFail: false,
			prepare:  func() *Collector { return prepareCollector(&mockClient{}, true) },
		},
		"does not list images or containers": {
			wantFail: false,
			prepare: func() *Collector {
				return prepareCollector(&mockClient{errOnImageList: true, errOnContainerList: true}, true)
			},
		},
		"fail on error on Info()": {
			wantFail: true,
			prepare:  func() *Collector { return prepareCollector(&mockClient{errOnInfo: true}, true) },
		},
		"fail on error creating Docker client": {
			wantFail: true,
			prepare:  func() *Collector { return prepareCollector(nil, true) },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	// The expected values are those of the V1 collector for the same mock data.
	summary := map[string]metrix.SampleValue{
		"containers_running":               4,
		"containers_paused":                5,
		"containers_exited":                6,
		"containers_healthy":               3,
		"containers_unhealthy":             1,
		"containers_not_running_unhealthy": 6,
		"containers_starting":              3,
		"containers_no_healthcheck":        4,
		"images_active":                    2,
		"images_dangling":                  1,
		"images_size":                      300,
	}
	// container17 has the netdata.cloud/ignore label: it is counted in the summary only.
	containers := []wantContainer{
		{name: "container1", image: "example/example:v1", state: "created", health: "healthy"},
		{name: "container2", image: "example/example:v1", state: "running", health: "healthy"},
		{name: "container3", image: "example/example:v1", state: "running", health: "healthy"},
		{name: "container4", image: "example/example:v2", state: "created", health: "not_running_unhealthy"},
		{name: "container5", image: "example/example:v2", state: "running", health: "unhealthy"},
		{name: "container6", image: "example/example:v2", state: "paused", health: "not_running_unhealthy"},
		{name: "container7", image: "example/example:v2", state: "restarting", health: "not_running_unhealthy"},
		{name: "container8", image: "example/example:v2", state: "removing", health: "not_running_unhealthy"},
		{name: "container9", image: "example/example:v2", state: "exited", health: "not_running_unhealthy"},
		{name: "container10", image: "example/example:v2", state: "dead", health: "not_running_unhealthy"},
		{name: "container11", image: "example/example:v3", state: "removing", health: "starting"},
		{name: "container12", image: "example/example:v3", state: "exited", health: "starting"},
		{name: "container13", image: "example/example:v3", state: "exited", health: "starting"},
		{name: "container14", image: "example/example:v4", state: "dead", health: "no_healthcheck"},
		{name: "container15", image: "example/example:v4", state: "dead", health: "no_healthcheck"},
		{name: "container16", image: "example/example:v4", state: "dead", health: "no_healthcheck"},
	}

	tests := map[string]struct {
		prepare    func() *Collector
		wantErr    bool
		wantSeries map[string]metrix.SampleValue
	}{
		"success": {
			prepare:    func() *Collector { return prepareCollector(&mockClient{}, true) },
			wantSeries: expectedSeries(summary, containers, true),
		},
		"success without container size": {
			prepare:    func() *Collector { return prepareCollector(&mockClient{}, false) },
			wantSeries: expectedSeries(summary, containers, false),
		},
		"container selector": {
			prepare: func() *Collector {
				collr := prepareCollector(&mockClient{}, false)
				collr.ContainerSelector = "container1 container5"
				return collr
			},
			wantSeries: expectedSeries(summary, []wantContainer{containers[0], containers[4]}, false),
		},
		"fail on error on Info()": {
			prepare: func() *Collector { return prepareCollector(&mockClient{errOnInfo: true}, true) },
			wantErr: true,
		},
		"fail on error on ImageList()": {
			prepare: func() *Collector { return prepareCollector(&mockClient{errOnImageList: true}, true) },
			wantErr: true,
		},
		"fail on error on ContainerList()": {
			prepare: func() *Collector { return prepareCollector(&mockClient{errOnContainerList: true}, true) },
			wantErr: true,
		},
		"fail on error creating Docker client": {
			prepare: func() *Collector { return prepareCollector(nil, true) },
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()
			require.NoError(t, collr.Init(context.Background()))

			got, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			if test.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantSeries, got)
		})
	}
}

func TestCollector_Collect_UnknownContainerState(t *testing.T) {
	m := &mockClient{containerState: "unknown"}
	collr := prepareCollector(m, false)
	collr.ContainerSelector = "container1"
	require.NoError(t, collr.Init(context.Background()))

	got, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	for _, state := range containerStates {
		key := series("container_state",
			"container_name", "container1",
			"container_state", state,
			"image", "example/example:v1",
		)
		assert.Equalf(t, metrix.SampleValue(0), got[key], "state %s", state)
	}
}

func TestCollector_Collect_ImagesRefresh(t *testing.T) {
	imagesV1 := []typesImage.Summary{
		{ID: "sha256:v1", Containers: -1, Size: 100},
		{ID: "sha256:unused", Containers: -1, Size: 200},
	}
	imagesV2 := []typesImage.Summary{{ID: "sha256:v2", Containers: -1, Size: 1000}}
	mxV1 := map[string]metrix.SampleValue{"images_active": 1, "images_dangling": 1, "images_size": 300}
	mxV2 := map[string]metrix.SampleValue{"images_active": 1, "images_dangling": 0, "images_size": 1000}

	type step struct {
		advance          time.Duration
		images           []typesImage.Summary
		errOnImageList   bool
		containerImageID string
		wantCalls        int
		wantImages       map[string]metrix.SampleValue
	}
	tests := map[string]struct {
		steps []step
	}{
		"cached values reported until the refresh interval elapses": {
			steps: []step{
				{images: imagesV1, wantCalls: 1, wantImages: mxV1},
				{advance: imagesRefreshEvery - time.Second, images: imagesV2, wantCalls: 1, wantImages: mxV1},
				{advance: time.Second, images: imagesV2, wantCalls: 2, wantImages: mxV2},
			},
		},
		"image usage follows the container list between refreshes": {
			steps: []step{
				{images: imagesV1, wantCalls: 1, wantImages: mxV1},
				{
					advance:          10 * time.Second,
					containerImageID: "sha256:removed",
					wantCalls:        1,
					wantImages:       map[string]metrix.SampleValue{"images_active": 0, "images_dangling": 2, "images_size": 300},
				},
				{advance: 10 * time.Second, wantCalls: 1, wantImages: mxV1},
			},
		},
		"collection interval longer than the refresh interval refreshes on every collection": {
			steps: []step{
				{images: imagesV1, wantCalls: 1, wantImages: mxV1},
				{advance: 2 * imagesRefreshEvery, images: imagesV2, wantCalls: 2, wantImages: mxV2},
				{advance: 2 * imagesRefreshEvery, images: imagesV1, wantCalls: 3, wantImages: mxV1},
			},
		},
		"failed refresh fails the collection and is retried on the next one": {
			steps: []step{
				{images: imagesV1, wantCalls: 1, wantImages: mxV1},
				{advance: imagesRefreshEvery, errOnImageList: true, wantCalls: 2, wantImages: nil},
				{advance: 10 * time.Second, images: imagesV2, wantCalls: 3, wantImages: mxV2},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := &mockClient{}
			collr := New()
			collr.newClient = prepareNewClientFunc(m)
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			collr.now = func() time.Time { return now }

			require.NoError(t, collr.Init(context.Background()))

			for i, s := range test.steps {
				now = now.Add(s.advance)
				m.images = s.images
				m.errOnImageList = s.errOnImageList
				m.containerImageID = s.containerImageID

				got, err := collecttest.CollectScalarSeries(collr)
				assert.Equalf(t, s.wantCalls, m.imageListCalls, "step %d: ImageList calls", i)
				if s.wantImages == nil {
					assert.Errorf(t, err, "step %d", i)
					continue
				}
				require.NoErrorf(t, err, "step %d", i)
				assert.Equalf(t, s.wantImages, imageSeries(got), "step %d: image metrics", i)
			}
		})
	}
}

func TestCollector_ChartCoverage(t *testing.T) {
	collr := prepareCollector(&mockClient{}, true)
	require.NoError(t, collr.Init(context.Background()))

	_, err := collecttest.CollectScalarSeries(collr)
	require.NoError(t, err)

	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

// The V1 charts (charts.go before the migration) are the oracle: chart IDs may change, but contexts, dimension names,
// presentation metadata and chart labels must survive.
func TestCollector_ChartContract(t *testing.T) {
	type chartContract struct {
		Title, Family, Units, Type string
		Priority                   int
		Labels                     map[string]string
		Dimensions                 []string
	}
	containerLabels := map[string]string{"container_name": "container2", "image": "example/example:v1"}
	want := map[string]chartContract{
		"docker.containers_state": {
			"Total number of Docker containers in various states", "containers", "containers", "stacked", 70000,
			map[string]string{}, []string{"running", "paused", "exited"},
		},
		"docker.containers_health_status": {
			"Total number of Docker containers in various health states", "containers", "containers", "line", 70001,
			map[string]string{}, []string{"healthy", "unhealthy", "not_running_unhealthy", "starting", "no_healthcheck"},
		},
		"docker.container_state": {
			"Docker container state", "containers", "state", "line", 70002,
			containerLabels, []string{"running", "paused", "exited", "created", "restarting", "removing", "dead"},
		},
		"docker.container_health_status": {
			"Docker container health status", "containers", "status", "line", 70003,
			containerLabels, []string{"healthy", "unhealthy", "not_running_unhealthy", "starting", "no_healthcheck"},
		},
		"docker.container_writeable_layer_size": {
			"Docker container writable layer size", "containers", "bytes", "line", 70004,
			containerLabels, []string{"writable_layer"},
		},
		"docker.images": {
			"Total number of Docker images in various states", "images", "images", "stacked", 70005,
			map[string]string{}, []string{"active", "dangling"},
		},
		"docker.images_size": {
			"Total size of all Docker images", "images", "bytes", "line", 70006,
			map[string]string{}, []string{"size"},
		},
	}

	collr := prepareCollector(&mockClient{}, true)
	collr.ContainerSelector = "container2"
	require.NoError(t, collr.Init(context.Background()))
	_, err := collecttest.CollectScalarSeries(collr)
	require.NoError(t, err)

	got := make(map[string]chartContract)
	contexts := make(map[string]string)
	for _, action := range preparePlan(t, newEngine(t, collr), collr).Actions {
		switch a := action.(type) {
		case chartengine.CreateChartAction:
			contexts[a.ChartID] = a.Meta.Context
			labels := a.Labels
			if labels == nil {
				labels = map[string]string{}
			}
			got[a.Meta.Context] = chartContract{
				a.Meta.Title, a.Meta.Family, a.Meta.Units, string(a.Meta.Type), a.Meta.Priority, labels, nil,
			}
		case chartengine.CreateDimensionAction:
			ctx := contexts[a.ChartID]
			v := got[ctx]
			v.Dimensions = append(v.Dimensions, a.Name)
			got[ctx] = v
			assert.Equalf(t, "absolute", string(a.Algorithm), "%s %s", ctx, a.Name)
			assert.Equalf(t, 1, a.Multiplier, "%s %s", ctx, a.Name)
			assert.Equalf(t, 1, a.Divisor, "%s %s", ctx, a.Name)
			assert.Falsef(t, a.Hidden, "%s %s", ctx, a.Name)
		}
	}
	assert.Equal(t, want, got)

	creator := collectorapi.DefaultRegistry["docker"]
	require.NotNil(t, creator.CreateV2)
	assert.Nil(t, creator.Create)
	assert.Equal(t, 10, creator.UpdateEvery)
}

// Container "foo" is a name prefix of "foo-bar" and "foo_state": removing "foo" must retire only its own charts
// (https://github.com/netdata/netdata/issues/24230).
func TestCollector_RemovesOnlyChartsOfRemovedContainer(t *testing.T) {
	m := &mockClient{}
	collr := prepareCollector(m, true)
	require.NoError(t, collr.Init(context.Background()))
	engine := newEngine(t, collr)

	created := make(map[string]map[string]string)
	removed := make(map[string]bool)
	cycle := func() {
		t.Helper()
		_, err := collecttest.CollectScalarSeries(collr)
		require.NoError(t, err)
		for _, action := range preparePlan(t, engine, collr).Actions {
			switch a := action.(type) {
			case chartengine.CreateChartAction:
				created[a.ChartID] = a.Labels
			case chartengine.RemoveChartAction:
				removed[a.ChartID] = true
			}
		}
	}

	m.containers = []typesContainer.Summary{
		{Names: []string{"/foo"}, State: "running", Image: "alpine", ImageID: "sha256:alpine"},
		{Names: []string{"/foo-bar"}, State: "running", Image: "alpine", ImageID: "sha256:alpine"},
		{Names: []string{"/foo_state"}, State: "running", Image: "alpine", ImageID: "sha256:alpine"},
	}
	cycle()
	require.Empty(t, removed)

	m.containers = m.containers[1:]
	// Keep collecting well beyond the chart expiry and the metric-store retention.
	for range 30 {
		cycle()
	}

	wantRemoved := make(map[string]bool)
	for chartID, labels := range created {
		if labels["container_name"] == "foo" {
			wantRemoved[chartID] = true
		}
	}
	assert.Len(t, wantRemoved, 3)
	assert.Equal(t, wantRemoved, removed)
}

type wantContainer struct {
	name, image, state, health string
}

func expectedSeries(
	summary map[string]metrix.SampleValue,
	containers []wantContainer,
	withSize bool,
) map[string]metrix.SampleValue {
	out := make(map[string]metrix.SampleValue, len(summary))
	for k, v := range summary {
		out[k] = v
	}
	for _, cntr := range containers {
		for _, state := range containerStates {
			key := series("container_state",
				"container_name", cntr.name,
				"container_state", state,
				"image", cntr.image,
			)
			out[key] = boolValue(state == cntr.state)
		}
		for _, health := range containerHealthStates {
			key := series("container_health_status",
				"container_health_status", health,
				"container_name", cntr.name,
				"image", cntr.image,
			)
			out[key] = boolValue(health == cntr.health)
		}
		if withSize {
			out[series("container_writable_layer_size", "container_name", cntr.name, "image", cntr.image)] = 123
		}
	}
	return out
}

func boolValue(v bool) metrix.SampleValue {
	if v {
		return 1
	}
	return 0
}

func series(name string, labels ...string) string {
	parts := make([]string, 0, len(labels)/2)
	for i := 0; i+1 < len(labels); i += 2 {
		parts = append(parts, labels[i]+"="+strconv.Quote(labels[i+1]))
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

func imageSeries(got map[string]metrix.SampleValue) map[string]metrix.SampleValue {
	images := make(map[string]metrix.SampleValue)
	for k, v := range got {
		if strings.HasPrefix(k, "images_") {
			images[k] = v
		}
	}
	return images
}

func newEngine(t *testing.T, c *Collector) *chartengine.Engine {
	t.Helper()
	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), 1))
	return engine
}

func preparePlan(t *testing.T, engine *chartengine.Engine, c *Collector) chartengine.Plan {
	t.Helper()
	attempt, err := engine.PreparePlan(c.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	return plan
}

func prepareCollector(m *mockClient, collectContainerSize bool) *Collector {
	collr := New()
	collr.CollectContainerSize = collectContainerSize
	collr.newClient = prepareNewClientFunc(m)
	return collr
}

func prepareNewClientFunc(m *mockClient) func(_ Config) (dockerClient, error) {
	if m == nil {
		return func(_ Config) (dockerClient, error) { return nil, errors.New("mock.newClient() error") }
	}
	return func(_ Config) (dockerClient, error) { return m, nil }
}

type mockClient struct {
	errOnInfo          bool
	errOnImageList     bool
	errOnContainerList bool
	closeCalled        bool
	images             []typesImage.Summary
	imageListCalls     int
	containerImageID   string
	containerState     typesContainer.ContainerState
	// containers, when set, replaces the default per-health-status lists: all of them are reported as having no
	// health check.
	containers []typesContainer.Summary
}

func (m *mockClient) Info(_ context.Context, _ docker.InfoOptions) (docker.SystemInfoResult, error) {
	if m.errOnInfo {
		return docker.SystemInfoResult{}, errors.New("mockClient.Info() error")
	}

	return docker.SystemInfoResult{
		Info: typesSystem.Info{
			ContainersRunning: 4,
			ContainersPaused:  5,
			ContainersStopped: 6,
		},
	}, nil
}

func (m *mockClient) ContainerList(_ context.Context, opts docker.ContainerListOptions) (docker.ContainerListResult, error) {
	if m.errOnContainerList {
		return docker.ContainerListResult{}, errors.New("mockClient.ContainerList() error")
	}

	values := opts.Filters["health"]
	if len(values) == 0 {
		return docker.ContainerListResult{}, errors.New("mockClient.ContainerList() error (expect 'health' filter)")
	}
	var status string
	for v := range values {
		status = v
		break
	}

	var containers []typesContainer.Summary

	if m.containers != nil {
		if typesContainer.HealthStatus(status) == typesContainer.NoHealthcheck {
			containers = append(containers, m.containers...)
		}
		return docker.ContainerListResult{Items: containers}, nil
	}

	switch typesContainer.HealthStatus(status) {
	case typesContainer.Healthy:
		containers = []typesContainer.Summary{
			{Names: []string{"container1"}, State: "created", Image: "example/example:v1", ImageID: "sha256:v1"},
			{Names: []string{"container2"}, State: "running", Image: "example/example:v1", ImageID: "sha256:v1"},
			{Names: []string{"container3"}, State: "running", Image: "example/example:v1", ImageID: "sha256:v1"},
		}
	case typesContainer.Unhealthy:
		containers = []typesContainer.Summary{
			{Names: []string{"container4"}, State: "created", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container5"}, State: "running", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container6"}, State: "paused", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container7"}, State: "restarting", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container8"}, State: "removing", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container9"}, State: "exited", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container10"}, State: "dead", Image: "example/example:v2", ImageID: "sha256:v2"},
		}
	case typesContainer.Starting:
		containers = []typesContainer.Summary{
			{Names: []string{"container11"}, State: "removing", Image: "example/example:v3", ImageID: "sha256:v3"},
			{Names: []string{"container12"}, State: "exited", Image: "example/example:v3", ImageID: "sha256:v3"},
			{Names: []string{"container13"}, State: "exited", Image: "example/example:v3", ImageID: "sha256:v3"},
		}
	case typesContainer.NoHealthcheck:
		containers = []typesContainer.Summary{
			{Names: []string{"container14"}, State: "dead", Image: "example/example:v4", ImageID: "sha256:v4"},
			{Names: []string{"container15"}, State: "dead", Image: "example/example:v4", ImageID: "sha256:v4"},
			{Names: []string{"container16"}, State: "dead", Image: "example/example:v4", ImageID: "sha256:v4"},
			{Names: []string{"container17"}, State: "dead", Image: "example/example:v5", ImageID: "sha256:v5",
				Labels: map[string]string{"netdata.cloud/ignore": "true"},
			},
		}
	}

	for i := range containers {
		if m.containerImageID != "" {
			containers[i].ImageID = m.containerImageID
		}
		if m.containerState != "" {
			containers[i].State = m.containerState
		}
		if opts.Size {
			containers[i].SizeRw = 123
			containers[i].SizeRootFs = 321
		}
	}

	return docker.ContainerListResult{Items: containers}, nil
}

func (m *mockClient) ImageList(_ context.Context, _ docker.ImageListOptions) (docker.ImageListResult, error) {
	m.imageListCalls++

	if m.errOnImageList {
		return docker.ImageListResult{}, errors.New("mockClient.ImageList() error")
	}
	if m.images != nil {
		return docker.ImageListResult{Items: m.images}, nil
	}

	// Containers is -1 when the negotiated API is older than 1.51.
	return docker.ImageListResult{Items: []typesImage.Summary{
		{ID: "sha256:v1", Containers: -1, Size: 100},
		{ID: "sha256:v5", Containers: -1, Size: 100},
		{ID: "sha256:unused", Containers: -1, Size: 100},
	}}, nil
}

func (m *mockClient) Close() error {
	m.closeCalled = true
	return nil
}
