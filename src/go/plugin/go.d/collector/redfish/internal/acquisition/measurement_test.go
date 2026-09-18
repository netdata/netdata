// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/require"
)

func measurementTestProject(t *testing.T, client *Client, nodes ...*graphNode) []measurement.Observation {
	t.Helper()
	projector := measurement.New(client.origin, "", ReadingProvenanceResolver(client.root, client.origin))
	resources := make([]*measurement.Resource, 0, len(nodes))
	for _, node := range nodes {
		resources = append(resources, &node.Resource)
	}
	result, err := projector.Project(resources, false, time.Unix(10, 0))
	require.NoError(t, err)
	return result.Observations
}

func measurementTestLabel(observation measurement.Observation, key string) string {
	for _, label := range observation.Labels {
		if label.Key == key {
			return label.Value
		}
	}
	return ""
}

func measurementTestReadings(observations []measurement.Observation) []measurement.Observation {
	var readings []measurement.Observation
	for _, observation := range observations {
		if observation.State == "" && measurementTestLabel(observation, "reading_key") != "" {
			readings = append(readings, observation)
		}
	}
	return readings
}

func measurementTestRequireValue(t *testing.T, observations []measurement.Observation, metric string, value float64) {
	t.Helper()
	var values []float64
	for _, observation := range observations {
		if observation.Metric == metric {
			values = append(values, observation.Value)
		}
	}
	require.Equal(t, []float64{value}, values, metric)
}

func measurementTestRequireAlarm(t *testing.T, observations []measurement.Observation, state string) {
	t.Helper()
	var alarms []string
	for _, observation := range observations {
		if strings.HasSuffix(observation.Metric, "_alarm") && measurementTestLabel(observation, "reading_key") != "" {
			alarms = append(alarms, observation.State)
		}
	}
	require.Equal(t, []string{state}, alarms, "source reading alarm")
}
