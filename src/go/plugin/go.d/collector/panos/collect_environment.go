// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

type environmentResult struct {
	Thermal     rawXMLSection `xml:"thermal"`
	Fan         rawXMLSection `xml:"fan"`
	Fans        rawXMLSection `xml:"fans"`
	Power       rawXMLSection `xml:"power"`
	PowerSupply rawXMLSection `xml:"power-supply"`
}

type rawXMLSection struct {
	InnerXML string `xml:",innerxml"`
}

type environmentEntry struct {
	Slot        string `xml:"slot"`
	Name        string `xml:"name"`
	Description string `xml:"description"`
	Alarm       string `xml:"alarm"`
	Inserted    string `xml:"Inserted"`
	Min         string `xml:"min"`
	Max         string `xml:"max"`
	DegreesC    string `xml:"DegreesC"`
	RPMs        string `xml:"RPMs"`
	Volts       string `xml:"Volts"`
}

type environmentSensor struct {
	kind  string
	entry environmentEntry
}

func (c *Collector) collectEnvironmentMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, environmentCommand)
	if err != nil {
		return false, fmt.Errorf("environment metricset: %s API call: %w", panosCommandName(environmentCommand), err)
	}

	env, err := parseEnvironment(body)
	if err != nil {
		return false, fmt.Errorf("environment metricset: %s response: %w", panosCommandName(environmentCommand), err)
	}

	sensors := env.sensors()

	var hasMetrics bool
	var errs []error
	for _, sensor := range sensors {
		entry := sensor.entry
		labels := environmentLabelValues(sensor.kind, entry)
		switch sensor.kind {
		case "temperature":
			alarm, err := parsePANOSAlarmField("environment temperature "+environmentSensorName(entry)+" alarm", entry.Alarm)
			if err != nil {
				errs = append(errs, err)
			} else {
				hasMetrics = true
				observeStateSetVec(c.metrics.env.sensorAlarm, alarmState(alarm), labels...)
			}
			value, err := parseRequiredPANOSDecimalField("environment temperature "+environmentSensorName(entry), entry.DegreesC, 1000)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			hasMetrics = true
			c.metrics.env.temperature.WithLabelValues(labels...).Observe(float64(value))
		case "fan":
			alarm, err := parsePANOSAlarmField("environment fan "+environmentSensorName(entry)+" alarm", entry.Alarm)
			if err != nil {
				errs = append(errs, err)
			} else {
				hasMetrics = true
				observeStateSetVec(c.metrics.env.sensorAlarm, alarmState(alarm), labels...)
			}
			value, err := parseRequiredPANOSIntField("environment fan "+environmentSensorName(entry)+" RPMs", entry.RPMs)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			hasMetrics = true
			c.metrics.env.fanSpeed.WithLabelValues(labels...).Observe(float64(value))
		case "voltage":
			alarm, err := parsePANOSAlarmField("environment voltage "+environmentSensorName(entry)+" alarm", entry.Alarm)
			if err != nil {
				errs = append(errs, err)
			} else {
				hasMetrics = true
				observeStateSetVec(c.metrics.env.sensorAlarm, alarmState(alarm), labels...)
			}
			value, err := parseRequiredPANOSDecimalField("environment voltage "+environmentSensorName(entry), entry.Volts, 1000)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			hasMetrics = true
			c.metrics.env.voltage.WithLabelValues(labels...).Observe(float64(value))
		case "power_supply":
			inserted, insertedErr := parsePANOSAffirmativeField("environment power supply "+environmentSensorName(entry)+" inserted", entry.Inserted)
			if insertedErr != nil {
				errs = append(errs, insertedErr)
			}
			alarm, alarmErr := parsePANOSAlarmField("environment power supply "+environmentSensorName(entry)+" alarm", entry.Alarm)
			if alarmErr != nil {
				errs = append(errs, alarmErr)
			}
			if insertedErr == nil {
				hasMetrics = true
				observeStateSetVec(c.metrics.env.powerSupplyPresence, boolState(inserted, "present", "absent"), labels...)
			}
			if alarmErr == nil {
				hasMetrics = true
				observeStateSetVec(c.metrics.env.powerSupplyAlarm, alarmState(alarm), labels...)
			}
		}
	}

	return hasMetrics, errors.Join(errs...)
}

type environmentMetrics struct {
	ThermalEntries     []environmentEntry
	FanEntries         []environmentEntry
	VoltageEntries     []environmentEntry
	PowerSupplyEntries []environmentEntry
}

func (m environmentMetrics) sensors() []environmentSensor {
	total := len(m.ThermalEntries) + len(m.FanEntries) + len(m.VoltageEntries) + len(m.PowerSupplyEntries)
	sensors := make([]environmentSensor, 0, total)
	for _, entry := range m.ThermalEntries {
		sensors = append(sensors, environmentSensor{kind: "temperature", entry: entry})
	}
	for _, entry := range m.FanEntries {
		sensors = append(sensors, environmentSensor{kind: "fan", entry: entry})
	}
	for _, entry := range m.VoltageEntries {
		sensors = append(sensors, environmentSensor{kind: "voltage", entry: entry})
	}
	for _, entry := range m.PowerSupplyEntries {
		sensors = append(sensors, environmentSensor{kind: "power_supply", entry: entry})
	}
	return sensors
}

func parseEnvironment(body []byte) (environmentMetrics, error) {
	var result environmentResult
	if err := decodePANOSResult(body, "PAN-OS environment response", &result); err != nil {
		return environmentMetrics{}, err
	}
	if !result.hasAnySection() {
		return environmentMetrics{}, missingPANOSResultError{expected: "<thermal>, <fan>, <fans>, <power>, or <power-supply>"}
	}

	thermal, err := decodeEnvironmentEntries(result.Thermal.InnerXML)
	if err != nil {
		return environmentMetrics{}, fmt.Errorf("thermal entries: %w", err)
	}
	fan, err := decodeEnvironmentFanEntries(result.Fan.InnerXML, result.Fans.InnerXML)
	if err != nil {
		return environmentMetrics{}, fmt.Errorf("fan entries: %w", err)
	}
	voltage, err := decodeEnvironmentEntries(result.Power.InnerXML)
	if err != nil {
		return environmentMetrics{}, fmt.Errorf("voltage entries: %w", err)
	}
	psu, err := decodeEnvironmentEntries(result.PowerSupply.InnerXML)
	if err != nil {
		return environmentMetrics{}, fmt.Errorf("power supply entries: %w", err)
	}

	return environmentMetrics{
		ThermalEntries:     thermal,
		FanEntries:         fan,
		VoltageEntries:     voltage,
		PowerSupplyEntries: psu,
	}, nil
}

func (r environmentResult) hasAnySection() bool {
	return firstNonEmpty(r.Thermal.InnerXML, r.Fan.InnerXML, r.Fans.InnerXML, r.Power.InnerXML, r.PowerSupply.InnerXML) != ""
}

func decodeEnvironmentFanEntries(sections ...string) ([]environmentEntry, error) {
	var entries []environmentEntry
	seen := make(map[string]bool)
	for _, section := range sections {
		decoded, err := decodeEnvironmentEntries(section)
		if err != nil {
			return nil, err
		}
		for _, entry := range decoded {
			key := environmentEntryIdentity(entry)
			if seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func environmentEntryIdentity(entry environmentEntry) string {
	return firstNonEmpty(entry.Slot, "unknown") + "\x00" + environmentSensorName(entry)
}

func decodeEnvironmentEntries(innerXML string) ([]environmentEntry, error) {
	if strings.TrimSpace(innerXML) == "" {
		return nil, nil
	}

	decoder := xml.NewDecoder(strings.NewReader(innerXML))
	var entries []environmentEntry

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return entries, nil
			}
			return nil, err
		}

		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "entry" {
			continue
		}

		var entry environmentEntry
		if err := decoder.DecodeElement(&entry, &start); err != nil {
			return nil, err
		}
		if firstNonEmpty(entry.Description, entry.Name, entry.Slot) == "" {
			continue
		}
		entries = append(entries, entry)
	}
}

func environmentSensorName(entry environmentEntry) string {
	return firstNonEmpty(entry.Description, entry.Name, "unknown")
}
