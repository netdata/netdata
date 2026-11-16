// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
)

type rateBaseline struct {
	Value      *big.Rat
	At         time.Time
	Epoch      string
	Multiplier float64
}

func (c *Projector) pruneRateBaselines(nodes []*Resource) {
	active := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		active[node.Key] = struct{}{}
	}
	c.rateMu.Lock()
	for key := range c.rateBaselines {
		nodeKey, _, ok := strings.Cut(key, "\x00")
		if !ok {
			continue
		}
		if _, exists := active[nodeKey]; !exists {
			delete(c.rateBaselines, key)
		}
	}
	c.rateMu.Unlock()
}

func (c *Projector) rateValue(
	key, exact string,
	multiplier float64,
	at time.Time,
	algorithm scalarAlgorithm,
	epoch string,
) (float64, bool) {
	// Callers supply values from bounded numeric decoding. Duration conversion
	// can produce a fraction longer than its source token.
	current, ok := new(big.Rat).SetString(exact)
	if !ok {
		return 0, false
	}
	c.rateMu.Lock()
	previous, exists := c.rateBaselines[key]
	if exists && !at.After(previous.At) {
		// A rejected observation cannot replace the last usable counter sample.
		c.rateMu.Unlock()
		return 0, false
	}
	c.rateBaselines[key] = rateBaseline{
		Value:      new(big.Rat).Set(current),
		At:         at,
		Epoch:      epoch,
		Multiplier: multiplier,
	}
	c.rateMu.Unlock()
	if !exists || previous.Epoch != epoch || previous.Multiplier != multiplier ||
		current.Cmp(previous.Value) < 0 {
		return 0, false
	}
	elapsed := at.Sub(previous.At).Seconds()
	delta := new(big.Rat).Sub(current, previous.Value)
	value, _ := delta.Float64()
	value *= multiplier / elapsed
	if algorithm == algorithmDurationPercent {
		value *= 100
		if value < 0 || value > 100 {
			return 0, false
		}
	}
	return value, isFinite(value)
}

func rateEpoch(document map[string]any) string {
	for _, path := range []string{
		"LifetimeStartDateTime",
		"CurrentPeriod.StartTime",
		"IntervalStartTime",
		"StartTime",
	} {
		if value, ok := stringValueAt(document, path); ok {
			return identity.TupleDigest("netdata:redfish:rate-epoch:v1", path, value)
		}
	}
	return ""
}

func (c *Projector) derivedPowerReading(
	node *Resource,
	source normalizedReading,
	observedAt time.Time,
) (normalizedReading, bool) {
	value, emit := c.rateValue(
		node.Key+"\x00reading-energy-rate\x00"+source.Key,
		source.SourceExact,
		source.SourceScale,
		observedAt,
		algorithmRate,
		readingRateEpoch(source),
	)
	if !emit {
		return normalizedReading{}, false
	}
	surface, ok := matchReading("power", "zero", "energy_rate", "energy_rate")
	if !ok {
		return normalizedReading{}, false
	}
	derived := source
	derived.IdentitySource = source.SourcePath + "\x00energy_rate"
	derived.Key = identity.Key("netdata:redfish:reading:v1", node.Key+"\x00"+derived.IdentitySource, 32)
	derived.Family = "power"
	derived.Units = "watts"
	derived.Role = "energy_rate"
	derived.Value = value
	derived.SourceExact = ""
	derived.Metric = surface.Metric
	derived.Primary = surface.Primary
	derived.AlarmMetric = ""
	derived.SemanticSourceClass = "energy_rate"
	derived.SourceAlarm = ""
	derived.Health = ""
	derived.DerivedHealth = ""
	derived.SourceAlarmDiagnostic = ""
	return derived, true
}

func readingRateEpoch(source normalizedReading) string {
	parts := []string{
		source.SourcePath,
		source.SourceType,
		source.SourceUnits,
		source.SourceBasis,
		source.Family,
		source.Units,
		source.Basis,
	}
	if source.DataSourceURI != nil {
		parts = append(parts, "reading_data_source_uri", *source.DataSourceURI)
	}
	if source.SensorResetTime != nil {
		parts = append(parts, "sensor_reset_time", strconv.FormatInt(*source.SensorResetTime, 10))
	}
	if source.LifetimeStartDateTime != nil {
		parts = append(parts, "lifetime_start_datetime", strconv.FormatInt(*source.LifetimeStartDateTime, 10))
	}
	return identity.TupleDigest("netdata:redfish:reading-rate-epoch:v1", parts...)
}
