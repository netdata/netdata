// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"fmt"
	"math"

	"github.com/bougou/go-ipmi/pkg/client"
	"github.com/bougou/go-ipmi/pkg/command/sensor"
	"github.com/bougou/go-ipmi/pkg/types"
)

// Get Sensor Reading response sizes. The state byte and its extension are optional.
const (
	readingSizeWithState         = 3
	readingSizeWithExtendedState = 4
)

// readingResponse records the response size because the SDK does not report
// which optional state bytes were present.
type readingResponse struct {
	sensor.GetSensorReadingResponse
	size int
}

func (r *readingResponse) Unpack(data []byte) error {
	r.size = len(data)
	return r.GetSensorReadingResponse.Unpack(data)
}

// readSensor reads the state and value of one inventory sensor. Completion-code
// errors and unavailable, incomplete or unconvertible readings degrade only
// this sensor; any other error fails the collection.
func (r *Reader) readSensor(ctx context.Context, d descriptor, warnings *collectionWarnings) (Sensor, error) {
	s := d.sensor
	if !d.supported {
		warnings.unsupported++
		return s, nil
	}

	ctx = client.WithCommandContext(ctx, (&client.CommandContext{}).
		WithResponderAddr(types.BMC_SA).
		WithResponderLUN(d.owner.LUN()))

	req := &sensor.GetSensorReadingRequest{
		SensorNumber: d.number,
	}
	var reading readingResponse
	if err := r.exchange(ctx, req, &reading); err != nil {
		if !isCompletionCodeError(err) {
			return Sensor{}, fmt.Errorf("read IPMI sensor: %w", err)
		}
		warnings.reading++
		return s, nil
	}
	if reading.ReadingUnavailable || reading.SensorScanningDisabled {
		warnings.reading++
		return s, nil
	}
	// Without the state byte the state is unknown, but the numeric reading is still valid.
	if reading.size < readingSizeWithState {
		warnings.reading++
	}
	s.State = sensorState(d.eventType, d.sensorType, reading)

	if !d.analog {
		return s, nil
	}
	if s.Unit == "" {
		warnings.conversion++
		return s, nil
	}
	value, ok, err := r.readValue(ctx, d, reading.Reading)
	if err != nil {
		return Sensor{}, err
	}
	if !ok {
		warnings.conversion++
		return s, nil
	}
	s.Value = &value
	return s, nil
}

// readValue converts a raw analog reading. Non-linear sensors report
// conversion factors for each raw reading; a completion-code error for them
// leaves no value.
func (r *Reader) readValue(ctx context.Context, d descriptor, raw uint8) (float64, bool, error) {
	factors, linearization := d.factors, d.linearization
	if linearization == types.LinearizationFunc_NonLinear {
		var res sensor.GetSensorReadingFactorsResponse
		req := &sensor.GetSensorReadingFactorsRequest{
			SensorNumber: d.number,
			Reading:      raw,
		}
		if err := r.exchange(ctx, req, &res); err != nil {
			if !isCompletionCodeError(err) {
				return 0, false, fmt.Errorf("read IPMI conversion factors: %w", err)
			}
			return 0, false, nil
		}
		factors, linearization = res.ReadingFactors, types.LinearizationFunc_Linear
	}
	value, ok := convertReading(raw, d.unit, factors, linearization)
	return value, ok, nil
}

// isCompletionCodeError reports whether the BMC answered with a failure
// completion code, as opposed to a transport or context failure.
func isCompletionCodeError(err error) bool {
	_, ok := types.IsResponseError(err)
	return ok
}

// convertReading applies the SDR conversion formula. Reserved and OEM
// non-linear linearizations and non-finite results have no value.
func convertReading(
	raw uint8,
	unit types.SensorUnit,
	factors types.ReadingFactors,
	linearization types.LinearizationFunc,
) (float64, bool) {
	if !unit.IsAnalog() || linearization > types.LinearizationFunc_CBRT {
		return 0, false
	}
	var value float64
	if linearization == types.LinearizationFunc_EXP10 {
		// The pinned SDK truncates the fractional EXP10 exponent (math.Pow10(int(x))).
		// Remove this after upgrading to a release that uses math.Pow(10, x).
		value = math.Pow(10, types.ConvertReading(raw, unit.AnalogDataFormat, factors, types.LinearizationFunc_Linear))
	} else {
		value = types.ConvertReading(raw, unit.AnalogDataFormat, factors, linearization)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}
