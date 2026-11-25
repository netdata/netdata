// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"errors"
	"maps"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
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
	collecttest.AssertConfigSchemaMatchesMetadataWith(t, "config_schema.json", "metadata.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		})
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config      Config
		dockerHost  string
		wantAddress string
		wantFail    bool
	}{
		"default config": {
			config:      New().Config,
			wantAddress: docker.DefaultDockerHost,
		},
		"DOCKER_HOST replaces the default address": {
			config:      New().Config,
			dockerHost:  "tcp://203.0.113.10:2375",
			wantAddress: "tcp://203.0.113.10:2375",
		},
		"DOCKER_HOST does not replace a configured address": {
			config: Config{
				Address: "unix:///run/user/1000/docker.sock",
			},
			dockerHost:  "tcp://203.0.113.10:2375",
			wantAddress: "unix:///run/user/1000/docker.sock",
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
			t.Setenv("DOCKER_HOST", test.dockerHost)
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
				return
			}
			require.NoError(t, collr.Init(context.Background()))
			assert.Equal(t, test.wantAddress, collr.Address)
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
		"fail on error on Info()": {
			wantFail: true,
			prepare: func() *Collector {
				return prepareCollector(&mockClient{
					errOnInfo: true,
				}, true)
			},
		},
		"fail on error on ImageList()": {
			wantFail: true,
			prepare: func() *Collector {
				return prepareCollector(&mockClient{
					errOnImageList: true,
				}, true)
			},
		},
		"fail on error on ContainerList()": {
			wantFail: true,
			prepare: func() *Collector {
				return prepareCollector(&mockClient{
					errOnContainerList: true,
				}, true)
			},
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

func TestCollector_Check_FirstCollectionReusesImageList(t *testing.T) {
	m := &mockClient{}
	collr := prepareCollector(m, false)
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	_, err := collecttest.CollectScalarSeries(collr)
	require.NoError(t, err)

	assert.Equal(t, 1, m.imageListCalls)
}

func TestCollector_Collect(t *testing.T) {
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
	// summaryWith is the summary of a custom container fixture: health counts start at zero.
	summaryWith := func(values map[string]metrix.SampleValue) map[string]metrix.SampleValue {
		out := maps.Clone(summary)
		for _, health := range containerHealthStates {
			out["containers_"+health] = 0
		}
		maps.Copy(out, values)
		return out
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
		"empty container selector charts every container": {
			prepare: func() *Collector {
				collr := prepareCollector(&mockClient{}, false)
				collr.ContainerSelector = ""
				return collr
			},
			wantSeries: expectedSeries(summary, containers, false),
		},
		"netdata.cloud/ignore label values": {
			prepare: func() *Collector {
				ignore := func(value string) map[string]string { return map[string]string{"netdata.cloud/ignore": value} }
				return prepareCollector(
					&mockClient{
						containers: map[typesContainer.HealthStatus][]typesContainer.Summary{
							typesContainer.NoHealthcheck: {
								{
									Names:   []string{"/yes"},
									State:   "running",
									Image:   "example/example:v1",
									ImageID: "sha256:v1",
									Labels:  ignore("yes"),
								},
								{
									Names:   []string{"/upper"},
									State:   "running",
									Image:   "example/example:v1",
									ImageID: "sha256:v1",
									Labels:  ignore("TRUE"),
								},
								{
									Names:   []string{"/charted"},
									State:   "running",
									Image:   "example/example:v1",
									ImageID: "sha256:v1",
									Labels:  ignore("false"),
								},
							},
						},
					},
					false,
				)
			},
			wantSeries: expectedSeries(
				summaryWith(map[string]metrix.SampleValue{
					"containers_no_healthcheck": 3,
					"images_active":             1,
					"images_dangling":           2,
				}),
				[]wantContainer{
					{name: "charted", image: "example/example:v1", state: "running", health: "no_healthcheck"},
				},
				false,
			),
		},
		"unknown container state leaves every state at zero": {
			prepare: func() *Collector {
				return prepareCollector(
					&mockClient{
						containers: map[typesContainer.HealthStatus][]typesContainer.Summary{
							typesContainer.NoHealthcheck: {
								{
									Names:   []string{"/container1"},
									State:   "unknown",
									Image:   "example/example:v1",
									ImageID: "sha256:v1",
								},
							},
						},
					},
					false,
				)
			},
			wantSeries: expectedSeries(
				summaryWith(map[string]metrix.SampleValue{
					"containers_no_healthcheck": 1,
					"images_active":             1,
					"images_dangling":           2,
				}),
				[]wantContainer{
					{name: "container1", image: "example/example:v1", state: "unknown", health: "no_healthcheck"},
				},
				false,
			),
		},
		"container replaced between list requests is charted once": {
			// "foo" was listed as unhealthy, then replaced by a container of another image before the "starting"
			// request listed it again. The totals count both listings; the per-container charts show the replacement.
			prepare: func() *Collector {
				return prepareCollector(
					&mockClient{
						containers: map[typesContainer.HealthStatus][]typesContainer.Summary{
							typesContainer.Unhealthy: {
								{
									Names:   []string{"/foo"},
									State:   "running",
									Image:   "example/example:v1",
									ImageID: "sha256:v1",
								},
							},
							typesContainer.Starting: {
								{
									Names:   []string{"/foo"},
									State:   "running",
									Image:   "example/example:v5",
									ImageID: "sha256:v5",
								},
							},
						},
					},
					true,
				)
			},
			wantSeries: expectedSeries(
				summaryWith(map[string]metrix.SampleValue{"containers_unhealthy": 1, "containers_starting": 1}),
				[]wantContainer{{name: "foo", image: "example/example:v5", state: "running", health: "starting"}},
				true,
			),
		},
		"fail on error on Info()": {
			prepare: func() *Collector {
				return prepareCollector(&mockClient{
					errOnInfo: true,
				}, true)
			},
			wantErr: true,
		},
		"fail on error on ImageList()": {
			prepare: func() *Collector {
				return prepareCollector(&mockClient{
					errOnImageList: true,
				}, true)
			},
			wantErr: true,
		},
		"fail on error on ContainerList()": {
			prepare: func() *Collector {
				return prepareCollector(&mockClient{
					errOnContainerList: true,
				}, true)
			},
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
					wantImages: map[string]metrix.SampleValue{
						"images_active":   0,
						"images_dangling": 2,
						"images_size":     300,
					},
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

type wantContainer struct {
	name, image, state, health string
}

func expectedSeries(
	summary map[string]metrix.SampleValue,
	containers []wantContainer,
	withSize bool,
) map[string]metrix.SampleValue {
	out := maps.Clone(summary)
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
	// images, when set, replaces defaultImages.
	images         []typesImage.Summary
	imageListCalls int
	// containers, when set, replaces defaultContainers: the containers each health filter returns.
	containers map[typesContainer.HealthStatus][]typesContainer.Summary
	// containerImageID, when set, replaces the image ID of every listed container.
	containerImageID string
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

func (m *mockClient) ContainerList(
	_ context.Context,
	opts docker.ContainerListOptions,
) (docker.ContainerListResult, error) {
	if m.errOnContainerList {
		return docker.ContainerListResult{}, errors.New("mockClient.ContainerList() error")
	}

	if !opts.All {
		return docker.ContainerListResult{}, errors.New(
			"mockClient.ContainerList() error (expect 'all' to include stopped containers)",
		)
	}

	values := opts.Filters["health"]
	if len(values) != 1 {
		return docker.ContainerListResult{}, errors.New("mockClient.ContainerList() error (expect one 'health' filter)")
	}
	var status typesContainer.HealthStatus
	for v := range values {
		status = typesContainer.HealthStatus(v)
	}

	byHealth := m.containers
	if byHealth == nil {
		byHealth = defaultContainers()
	}
	containers := append([]typesContainer.Summary(nil), byHealth[status]...)

	for i := range containers {
		if m.containerImageID != "" {
			containers[i].ImageID = m.containerImageID
		}
		if opts.Size {
			containers[i].SizeRw = 123
		}
	}

	return docker.ContainerListResult{
		Items: containers,
	}, nil
}

func (m *mockClient) ImageList(_ context.Context, _ docker.ImageListOptions) (docker.ImageListResult, error) {
	m.imageListCalls++

	if m.errOnImageList {
		return docker.ImageListResult{}, errors.New("mockClient.ImageList() error")
	}
	if m.images != nil {
		return docker.ImageListResult{
			Items: m.images,
		}, nil
	}
	return docker.ImageListResult{
		Items: defaultImages(),
	}, nil
}

func (m *mockClient) Close() error {
	m.closeCalled = true
	return nil
}

func defaultImages() []typesImage.Summary {
	// Containers is -1 when the negotiated API is older than 1.51.
	return []typesImage.Summary{
		{ID: "sha256:v1", Containers: -1, Size: 100},
		{ID: "sha256:v5", Containers: -1, Size: 100},
		{ID: "sha256:unused", Containers: -1, Size: 100},
	}
}

func defaultContainers() map[typesContainer.HealthStatus][]typesContainer.Summary {
	return map[typesContainer.HealthStatus][]typesContainer.Summary{
		typesContainer.Healthy: {
			{Names: []string{"container1"}, State: "created", Image: "example/example:v1", ImageID: "sha256:v1"},
			{Names: []string{"container2"}, State: "running", Image: "example/example:v1", ImageID: "sha256:v1"},
			{Names: []string{"container3"}, State: "running", Image: "example/example:v1", ImageID: "sha256:v1"},
		},
		typesContainer.Unhealthy: {
			{Names: []string{"container4"}, State: "created", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container5"}, State: "running", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container6"}, State: "paused", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container7"}, State: "restarting", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container8"}, State: "removing", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container9"}, State: "exited", Image: "example/example:v2", ImageID: "sha256:v2"},
			{Names: []string{"container10"}, State: "dead", Image: "example/example:v2", ImageID: "sha256:v2"},
		},
		typesContainer.Starting: {
			{Names: []string{"container11"}, State: "removing", Image: "example/example:v3", ImageID: "sha256:v3"},
			{Names: []string{"container12"}, State: "exited", Image: "example/example:v3", ImageID: "sha256:v3"},
			{Names: []string{"container13"}, State: "exited", Image: "example/example:v3", ImageID: "sha256:v3"},
		},
		typesContainer.NoHealthcheck: {
			{Names: []string{"container14"}, State: "dead", Image: "example/example:v4", ImageID: "sha256:v4"},
			{Names: []string{"container15"}, State: "dead", Image: "example/example:v4", ImageID: "sha256:v4"},
			{Names: []string{"container16"}, State: "dead", Image: "example/example:v4", ImageID: "sha256:v4"},
			{Names: []string{"container17"}, State: "dead", Image: "example/example:v5", ImageID: "sha256:v5",
				Labels: map[string]string{"netdata.cloud/ignore": "true"},
			},
		},
	}
}
