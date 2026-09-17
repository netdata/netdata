// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
)

// Projector owns one job's measurement history. The collector invokes Project once
// after acquisition and session recovery settle, never for a discarded attempt.
// Calls to Project must be serialized, as they are by the collector lifecycle.
type Projector struct {
	origin            string
	endpointJob       string
	resolveProvenance func(baseURI, raw string) (string, bool)
	rateMu            sync.Mutex
	rateBaselines     map[string]rateBaseline
	identities        identity.Registry
}

// New binds endpoint labels and the acquisition layer's pure URI policy. A nil
// resolver retains source-path identity for readings without canonical provenance.
func New(origin, job string, resolveProvenance func(baseURI, raw string) (string, bool)) *Projector {
	p := &Projector{
		origin:            origin,
		endpointJob:       job,
		resolveProvenance: resolveProvenance,
	}
	p.rateBaselines = make(map[string]rateBaseline)
	return p
}

// Result contains observations and diagnostic messages for the collector to publish
// and bound together with acquisition diagnostics. No input resource is modified.
type Result struct {
	Observations []Observation
	Diagnostics  []string
}

type Observation struct {
	Metric string
	Value  float64
	State  string
	Labels []metrix.Label
}

// Project converts the final acquired resources. complete is graph completeness,
// which authorizes pruning rate baselines for resources no longer present.
func (c *Projector) Project(nodes []*Resource, complete bool, observedAt time.Time) (Result, error) {
	var result Result
	if complete {
		c.pruneRateBaselines(nodes)
	}
	readings := make(map[string][]normalizedReading, len(nodes))
	for _, node := range nodes {
		readings[node.Key] = c.readingsForNode(node, observedAt)
	}
	if err := c.validateAndRegisterReadingIdentities(readings); err != nil {
		return result, err
	}
	var observations []Observation
	for _, node := range nodes {
		values := c.scalarValues(node, observedAt)
		if value, present, diagnostic := managerClockValue(node); present {
			if diagnostic != "" {
				result.addDiagnostic(fmt.Sprintf("manager %s clock offset: %s", node.URI, diagnostic))
			}
			if value.Valid {
				values = append(values, value)
			}
		}
		observations = append(observations, c.statusObservations(node)...)
		labels := c.metricLabels(node, nil)
		for _, value := range values {
			for _, failure := range value.SourceFailures {
				result.addDiagnostic(failure)
			}
			if value.Emit {
				observations = append(
					observations,
					Observation{
						Metric: value.Descriptor.Metric,
						Value:  value.Value,
						Labels: labels,
					},
				)
			}
		}
		observations = append(observations, c.flagObservations(node, flagValues(node))...)
		for _, reading := range readings[node.Key] {
			result.addDiagnostic(reading.SourceAlarmDiagnostic)
			if reading.Valid || reading.SourceAlarm != "" {
				observations = append(observations, c.readingObservations(node, reading)...)
			}
		}
	}
	result.Observations = observations
	return result, nil
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

func (c *Projector) validateAndRegisterReadingIdentities(
	readings map[string][]normalizedReading,
) error {
	if err := validateReadingIdentities(readings); err != nil {
		return fmt.Errorf("%w: %v", identity.ErrIntegrity, err)
	}
	bindings := make([]identity.Binding, 0)
	for nodeKey, values := range readings {
		for _, reading := range values {
			bindings = append(bindings, identity.Binding{
				Domain:   "reading",
				Key:      reading.Key,
				Preimage: nodeKey + "\x00" + reading.IdentitySource,
			})
		}
	}
	if err := c.identities.Register(bindings); err != nil {
		return fmt.Errorf("%w: Redfish reading-key collision", err)
	}
	return nil
}

func (r *Result) addDiagnostic(message string) {
	if message != "" {
		r.Diagnostics = append(r.Diagnostics, message)
	}
}
