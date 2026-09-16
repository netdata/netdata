// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type hardwareObservation struct {
	Metric string
	Value  float64
	State  string
	Labels []metrix.Label
}

func (c *protocolClient) hardwareSurface(graph *resourceGraph, observedAt time.Time) ([]hardwareObservation, error) {
	nodes := graph.emittedNodes()
	if graph.Complete {
		c.pruneRateBaselines(nodes)
	}
	readings := make(map[string][]normalizedReading, len(nodes))
	for _, node := range nodes {
		readings[node.Key] = c.readingsForNode(node, observedAt)
	}
	if err := c.validateAndRegisterReadingIdentities(readings); err != nil {
		return nil, err
	}
	var observations []hardwareObservation
	for _, node := range nodes {
		values := c.scalarValues(node, observedAt)
		if value, present, diagnostic := managerClockValue(node); present {
			if diagnostic != "" {
				graph.addDiagnostic(fmt.Sprintf("manager %s clock offset: %s", node.URI, diagnostic))
			}
			if value.Valid {
				values = append(values, value)
			}
		}
		observations = append(observations, c.statusObservations(node)...)
		labels := c.metricLabels(node, nil)
		for _, value := range values {
			for _, failure := range value.SourceFailures {
				graph.addDiagnostic(failure)
			}
			if value.Emit {
				observations = append(
					observations,
					hardwareObservation{
						Metric: value.Descriptor.Metric,
						Value:  value.Value,
						Labels: labels,
					},
				)
			}
		}
		observations = append(observations, c.flagObservations(node, flagValues(node))...)
		for _, reading := range readings[node.Key] {
			graph.addDiagnostic(reading.SourceAlarmDiagnostic)
			if reading.Valid || reading.SourceAlarm != "" {
				observations = append(observations, c.readingObservations(node, reading)...)
			}
		}
	}
	return observations, nil
}

func validateReadingIdentities(readings map[string][]normalizedReading) error {
	seen := make(map[string]string)
	for nodeKey, values := range readings {
		for _, reading := range values {
			preimage := nodeKey + "\x00" + reading.IdentitySource
			if previous, exists := seen[reading.Key]; exists && previous != preimage {
				return fmt.Errorf("Redfish reading-key collision for %s", reading.Key)
			}
			seen[reading.Key] = preimage
		}
	}
	return nil
}

func (c *protocolClient) validateAndRegisterReadingIdentities(
	readings map[string][]normalizedReading,
) error {
	if err := validateReadingIdentities(readings); err != nil {
		return fmt.Errorf("%w: %v", errIdentityIntegrity, err)
	}
	bindings := make([]identityBinding, 0)
	for nodeKey, values := range readings {
		for _, reading := range values {
			bindings = append(bindings, identityBinding{
				Domain:   "reading",
				Key:      reading.Key,
				Preimage: nodeKey + "\x00" + reading.IdentitySource,
			})
		}
	}
	if err := c.identities.register(bindings); err != nil {
		return fmt.Errorf("%w: Redfish reading-key collision", err)
	}
	return nil
}
