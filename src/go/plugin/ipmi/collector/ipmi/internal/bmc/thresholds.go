// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"

	"github.com/bougou/go-ipmi/pkg/command/sensor"
	"github.com/bougou/go-ipmi/pkg/types"
)

// Threshold positions in rawThresholds, lowest first.
const (
	thresholdLNR = iota
	thresholdLCR
	thresholdLNC
	thresholdUNC
	thresholdUCR
	thresholdUNR
	thresholdCount
)

// rawThresholds are threshold bytes before conversion and whether each is readable.
type rawThresholds struct {
	readable [thresholdCount]bool
	raw      [thresholdCount]uint8
}

// sdrThresholds returns the threshold fields of a full record. The SDK parses
// the two threshold masks swapped: its Settable flags hold the record's
// readable threshold mask (IPMI 2.0 Table 43-1 byte 19, as FreeIPMI and
// ipmitool read it).
func sdrThresholds(f *types.SDRFull) rawThresholds {
	t := f.Mask.Threshold
	return rawThresholds{
		readable: [thresholdCount]bool{
			t.LNR.Settable, t.LCR.Settable, t.LNC.Settable, t.UNC.Settable, t.UCR.Settable, t.UNR.Settable,
		},
		raw: [thresholdCount]uint8{f.LNR_Raw, f.LCR_Raw, f.LNC_Raw, f.UNC_Raw, f.UCR_Raw, f.UNR_Raw},
	}
}

// hasThresholds reports whether the sensor's thresholds can be shown: a
// threshold sensor whose raw values convert like its readings. Non-linear
// sensors need factors for each raw value and, as in FreeIPMI, have none.
func (d *descriptor) hasThresholds() bool {
	return d.supported && d.eventType == eventTypeThreshold && d.analog && d.sensor.Unit != "" &&
		d.linearization <= types.LinearizationFunc_CBRT && d.thresholdAccess != types.SensorThresholdAccess_No
}

// readThresholds sets the thresholds of the inventory's threshold sensors and
// returns how many could not be read. Values come from Get Sensor Thresholds,
// because the SDR can disagree with what the BMC uses; only fixed, unreadable
// thresholds come from the SDR, which holds their values. A completion-code
// error leaves that sensor without thresholds; any other failure stops the
// reads for this inventory. Neither fails the collection, because sensor
// states come from the readings; a cancellation surfaces in Collect.
func (r *Reader) readThresholds(ctx context.Context, inventory []descriptor) (unavailable int) {
	stopped := false
	for i := range inventory {
		d := &inventory[i]
		switch {
		case !d.hasThresholds():
			continue
		case d.thresholdAccess == types.SensorThresholdAccess_Fixed:
			d.sensor.Thresholds = d.convertThresholds(d.sdrThresholds)
			continue
		case stopped:
			unavailable++
			continue
		}

		live, err := r.readLiveThresholds(ctx, d)
		if err != nil {
			unavailable++
			stopped = !isCompletionCodeError(err)
			continue
		}
		d.sensor.Thresholds = d.convertThresholds(live)
	}
	return unavailable
}

func (r *Reader) readLiveThresholds(ctx context.Context, d *descriptor) (rawThresholds, error) {
	var res sensor.GetSensorThresholdsResponse
	req := &sensor.GetSensorThresholdsRequest{
		SensorNumber: d.number,
	}
	if err := r.exchange(ownerContext(ctx, d.owner), req, &res); err != nil {
		return rawThresholds{}, err
	}
	return rawThresholds{
		readable: [thresholdCount]bool{
			res.LNR_Readable, res.LCR_Readable, res.LNC_Readable, res.UNC_Readable, res.UCR_Readable, res.UNR_Readable,
		},
		raw: [thresholdCount]uint8{res.LNR_Raw, res.LCR_Raw, res.LNC_Raw, res.UNC_Raw, res.UCR_Raw, res.UNR_Raw},
	}, nil
}

// convertThresholds converts the readable thresholds with the reading's formula.
func (d *descriptor) convertThresholds(t rawThresholds) Thresholds {
	var values [thresholdCount]*float64
	for i, readable := range t.readable {
		if !readable {
			continue
		}
		if v, ok := convertReading(t.raw[i], d.unit, d.factors, d.linearization); ok {
			values[i] = &v
		}
	}
	return Thresholds{
		LowerNonRecoverable: values[thresholdLNR],
		LowerCritical:       values[thresholdLCR],
		LowerNonCritical:    values[thresholdLNC],
		UpperNonCritical:    values[thresholdUNC],
		UpperCritical:       values[thresholdUCR],
		UpperNonRecoverable: values[thresholdUNR],
	}
}
