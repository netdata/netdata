// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"strings"
)

const UnitRegistryVersion = "v1"

type rationalScale struct {
	multiplier int64
	divisor    int64
}

type compiledComponent struct {
	source        Component
	algorithm     string
	effectiveRate string
	canonicalUnit string
	scale         rationalScale
}

type registeredDisplay struct {
	quantity      string
	object        string
	effectiveRate string
	unit          string
	scale         rationalScale
}

type registeredSourceRate struct {
	effectiveRate string
	scale         rationalScale
}

var sourceUnitRegistry = map[string]map[string]rationalScale{
	"count": {"one": newRationalScale(1, 1)},
	"data": {
		"byte":    newRationalScale(1, 1),
		"megabit": newRationalScale(125_000, 1),
	},
	"duration": {
		"second":      newRationalScale(1, 1),
		"millisecond": newRationalScale(1, 1_000),
		"hour":        newRationalScale(3600, 1),
	},
	"duration_squared": {
		"second_squared":      newRationalScale(1, 1),
		"microsecond_squared": newRationalScale(1, 1_000_000_000_000),
	},
	"timestamp": {"unix_second": newRationalScale(1, 1)},
	"ratio": {
		"one":       newRationalScale(1, 1),
		"percent":   newRationalScale(1, 100),
		"per_mille": newRationalScale(1, 1_000),
	},
	"currency":    {"usd": newRationalScale(1, 1)},
	"temperature": {"celsius": newRationalScale(1, 1)},
	"frequency":   {"hertz": newRationalScale(1, 1)},
	"state":       {"one": newRationalScale(1, 1)},
}

var sourceRateRegistry = map[string]registeredSourceRate{
	"none": {
		effectiveRate: "none",
		scale:         newRationalScale(1, 1),
	},
	"per_second": {
		effectiveRate: "per_second",
		scale:         newRationalScale(1, 1),
	},
	"per_minute": {
		effectiveRate: "per_second",
		scale:         newRationalScale(1, 60),
	},
}

var displayRegistry = map[string]registeredDisplay{
	"cpu_cores": {
		quantity:      "duration",
		object:        "process_cpu_time",
		effectiveRate: "per_second",
		unit:          "cores",
		scale:         newRationalScale(1, 1),
	},
	"file_descriptors": {
		quantity:      "count",
		object:        "file_descriptors",
		effectiveRate: "none",
		unit:          "fds",
		scale:         newRationalScale(1, 1),
	},
	"requests_per_minute": {
		quantity:      "count",
		object:        "requests",
		effectiveRate: "per_second",
		unit:          "requests/min",
		scale:         newRationalScale(60, 1),
	},
	"tokens_per_minute": {
		quantity:      "count",
		object:        "tokens",
		effectiveRate: "per_second",
		unit:          "tokens/min",
		scale:         newRationalScale(60, 1),
	},
	"request_failures": {
		quantity:      "count",
		object:        "requests",
		effectiveRate: "per_second",
		unit:          "failures/s",
		scale:         newRationalScale(1, 1),
	},
	"duration_hours": {
		quantity:      "duration",
		effectiveRate: "none",
		unit:          "hours",
		scale:         newRationalScale(1, 3600),
	},
	"percentage": {
		quantity:      "ratio",
		effectiveRate: "none",
		unit:          "percentage",
		scale:         newRationalScale(100, 1),
	},
	"network_bits_per_second": {
		quantity:      "data",
		object:        "network_data",
		effectiveRate: "per_second",
		unit:          "bits/s",
		scale:         newRationalScale(8, 1),
	},
	"gflops_per_second_per_gpu": {
		quantity:      "count",
		object:        "floating_point_operations_per_gpu",
		effectiveRate: "per_second",
		unit:          "GFLOP/s/GPU",
		scale:         newRationalScale(1, 1_000_000_000),
	},
	"gigabytes_per_second_per_gpu": {
		quantity:      "data",
		object:        "data_per_gpu",
		effectiveRate: "per_second",
		unit:          "GB/s/GPU",
		scale:         newRationalScale(1, 1_000_000_000),
	},
	"duration_milliseconds": {
		quantity:      "duration",
		effectiveRate: "none",
		unit:          "milliseconds",
		scale:         newRationalScale(1000, 1),
	},
	"per_mille": {
		quantity:      "ratio",
		effectiveRate: "none",
		unit:          "per mille",
		scale:         newRationalScale(1000, 1),
	},
	"duration_squared_microseconds_per_second": {
		quantity:      "duration_squared",
		effectiveRate: "per_second",
		unit:          "microseconds²/s",
		scale:         newRationalScale(1_000_000_000_000, 1),
	},
	"network_megabits_per_second": {
		quantity:      "data",
		object:        "network_data",
		effectiveRate: "per_second",
		unit:          "Mbps",
		scale:         newRationalScale(8, 1_000_000),
	},
	"placement_groups": {
		quantity:      "count",
		object:        "placement_groups",
		effectiveRate: "none",
		unit:          "PGs",
		scale:         newRationalScale(1, 1),
	},
	"virtual_machines": {
		quantity:      "count",
		object:        "vms",
		effectiveRate: "none",
		unit:          "VMs",
		scale:         newRationalScale(1, 1),
	},
	"osds": {
		quantity:      "count",
		object:        "osds",
		effectiveRate: "none",
		unit:          "OSDs",
		scale:         newRationalScale(1, 1),
	},
}

func compileComponentUnit(field string, component Component) (compiledComponent, error) {
	bases := sourceUnitRegistry[component.Unit.Quantity]
	scale, ok := bases[component.Unit.Base]
	if !ok {
		return compiledComponent{}, fmt.Errorf("%s.unit base %q is not registered for quantity %q",
			field, component.Unit.Base, component.Unit.Quantity)
	}
	rate, ok := sourceRateRegistry[component.Unit.Rate]
	if !ok {
		return compiledComponent{}, fmt.Errorf("%s.unit rate %q is not registered", field, component.Unit.Rate)
	}
	effectiveRate := rate.effectiveRate
	scale = scale.multiply(rate.scale)
	algorithm := "absolute"
	if component.Lifecycle.Kind == "cumulative" {
		algorithm = "incremental"
		effectiveRate = "per_second"
	}
	unit, err := canonicalUnit(component.Unit.Quantity, component.Unit.Base, component.Unit.Object, effectiveRate)
	if err != nil {
		return compiledComponent{}, fmt.Errorf("%s.unit: %w", field, err)
	}
	return compiledComponent{
		source:        component,
		algorithm:     algorithm,
		effectiveRate: effectiveRate,
		canonicalUnit: unit,
		scale:         scale,
	}, nil
}

func compileSignalComponents(
	field string,
	components map[string]Component,
) (map[string]compiledComponent, error) {
	result := make(map[string]compiledComponent, len(components))
	for _, id := range sortedMapKeys(components) {
		component, err := compileComponentUnit(field+"."+id, components[id])
		if err != nil {
			return nil, err
		}
		result[id] = component
	}
	return result, nil
}

func canonicalUnit(quantity, base, object, rate string) (string, error) {
	var unit string
	switch quantity {
	case "count":
		unit = strings.ReplaceAll(object, "_", " ")
	case "data":
		unit = "bytes"
	case "duration":
		unit = "seconds"
	case "duration_squared":
		unit = "seconds²"
	case "timestamp":
		unit = "seconds since epoch"
	case "ratio":
		unit = "ratio"
	case "currency":
		switch base {
		case "usd":
			unit = "USD"
		default:
			return "", fmt.Errorf("currency base %q has no canonical formatter", base)
		}
	case "temperature":
		unit = "Celsius"
	case "frequency":
		unit = "Hz"
	case "state":
		unit = "{status}"
	default:
		return "", fmt.Errorf("quantity %q has no canonical formatter", quantity)
	}
	if rate == "per_second" {
		unit += "/s"
	}
	return unit, nil
}

func resolveRegisteredDisplay(
	field string,
	component compiledComponent,
	convention string,
) (string, rationalScale, error) {
	display, ok := displayRegistry[convention]
	if !ok {
		return "", rationalScale{}, fmt.Errorf("%s convention %q is not registered", field, convention)
	}
	unit := component.source.Unit
	if display.quantity != unit.Quantity ||
		display.object != "" && display.object != unit.Object ||
		display.effectiveRate != component.effectiveRate {
		return "", rationalScale{}, fmt.Errorf(
			"%s convention %q requires (%s,%s,%s), got (%s,%s,%s)",
			field,
			convention,
			display.quantity,
			display.object,
			display.effectiveRate,
			unit.Quantity,
			unit.Object,
			component.effectiveRate,
		)
	}
	return display.unit, component.scale.multiply(display.scale), nil
}

func newRationalScale(multiplier, divisor int64) rationalScale {
	if divisor == 0 {
		panic("zero rational scale divisor")
	}
	if divisor < 0 {
		multiplier = -multiplier
		divisor = -divisor
	}
	divisorGCD := gcd64(abs64(multiplier), divisor)
	return rationalScale{multiplier: multiplier / divisorGCD, divisor: divisor / divisorGCD}
}

func (r rationalScale) multiply(other rationalScale) rationalScale {
	return newRationalScale(r.multiplier*other.multiplier, r.divisor*other.divisor)
}

func gcd64(left, right int64) int64 {
	for right != 0 {
		left, right = right, left%right
	}
	if left == 0 {
		return 1
	}
	return left
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
