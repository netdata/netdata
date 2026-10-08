// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/bougou/go-ipmi/pkg/client"
	"github.com/bougou/go-ipmi/pkg/command/app"
	"github.com/bougou/go-ipmi/pkg/command/sensor"
	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
)

var testNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

const fakeSELEntries = 659

// fakeBMC answers commands with encoded response bytes, so every response goes
// through the SDK's wire decoding. Record IDs are 1-based positions in records.
type fakeBMC struct {
	records     [][]byte         // SDR records in chain order
	readings    map[uint8][]byte // Get Sensor Reading response by sensor number
	readingErrs map[uint8]error
	repoErr     error
	selErr      error
	closeErr    error
	// additionTime is the repository's most recent addition timestamp.
	additionTime uint32
	// recordCount overrides the repository record count; zero reports len(records).
	recordCount int
	// refuseWholeReads makes Get SDR require partial reads.
	refuseWholeReads bool
	// hideReserveSupport reports no Reserve SDR Repository support.
	hideReserveSupport bool
	// partialLength, when set, decides how many requested bytes a partial Get SDR returns.
	partialLength func(offset, requested int) int
	// wholeLength, when set, decides how many bytes of a record a whole-record Get SDR returns.
	wholeLength func(size int) int
	// onConnect and onExchange run before Connect and each command; a non-nil
	// error is returned instead.
	onConnect  func() error
	onExchange func(types.Request) error

	requests []types.Request
	connects int
	closes   int
}

// newFakeBMC returns a BMC with a threshold temperature sensor (number 10) and
// a discrete processor presence sensor (number 11), both healthy. Its sentinel
// repository timestamp never changes.
func newFakeBMC() *fakeBMC {
	return &fakeBMC{
		records: [][]byte{
			fullRecord(1, 10, "CPU Temp"),
			compactRecord(2, 11, 0x07, "CPU Presence"),
		},
		readings: map[uint8][]byte{
			10: {25, 0xc0, 0x00},      // 25, events and scanning enabled, no threshold crossed
			11: {0, 0xc0, 0x80, 0x00}, // processor presence detected
		},
		readingErrs:  map[uint8]error{},
		additionTime: 0xffffffff,
	}
}

// Expected sensors of newFakeBMC's records.
func wantCPUTemp(state string, value *float64) Sensor {
	return Sensor{
		Key:       "i1_n10_t2_u1_CPU Temp",
		Name:      "CPU Temp",
		Type:      "Temperature",
		Component: "Processor",
		Unit:      UnitCelsius,
		State:     state,
		Value:     value,
	}
}

func wantCPUPresence(state string) Sensor {
	return Sensor{
		Key:       "i2_n11_t255_u0_CPU Presence",
		Name:      "CPU Presence",
		Type:      "Processor",
		Component: "Processor",
		State:     state,
	}
}

func newTestReader(fake *fakeBMC) *Reader {
	r := New(Config{
		Timeout: time.Second,
	})
	r.newTransport = func() (transport, error) { return fake, nil }
	r.now = func() time.Time { return testNow }
	return r
}

func (f *fakeBMC) Connect(context.Context) error {
	if f.onConnect != nil {
		if err := f.onConnect(); err != nil {
			return err
		}
	}
	f.connects++
	return nil
}

func (f *fakeBMC) Close(context.Context) error { f.closes++; return f.closeErr }

func (f *fakeBMC) Exchange(ctx context.Context, req types.Request, res types.Response) error {
	f.requests = append(f.requests, req)
	if f.onExchange != nil {
		if err := f.onExchange(req); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := f.respond(ctx, req)
	if err != nil {
		return err
	}
	if err := res.Unpack(data); err != nil {
		return types.NewResponseError(0, err.Error())
	}
	return nil
}

func (f *fakeBMC) respond(ctx context.Context, req types.Request) ([]byte, error) {
	switch q := req.(type) {
	case *app.GetDeviceIDRequest:
		return make([]byte, 15), nil
	case *storage.GetSDRRepoInfoRequest:
		if f.repoErr != nil {
			return nil, f.repoErr
		}
		data := make([]byte, 14)
		data[0] = 0x51 // SDR version
		count := len(f.records)
		if f.recordCount > 0 {
			count = f.recordCount
		}
		binary.LittleEndian.PutUint16(data[1:], uint16(count))
		binary.LittleEndian.PutUint32(data[5:], f.additionTime)
		if !f.hideReserveSupport {
			data[13] = 0x02 // Reserve SDR Repository supported
		}
		return data, nil
	case *storage.ReserveSDRRepoRequest:
		return []byte{1, 0}, nil
	case *storage.GetSDRRequest:
		return f.respondSDR(q)
	case *sensor.GetSensorReadingRequest:
		if client.GetCommandContext(ctx) == nil {
			return nil, errors.New("missing owner addressing context")
		}
		if err := f.readingErrs[q.SensorNumber]; err != nil {
			return nil, err
		}
		return f.readings[q.SensorNumber], nil
	case *sensor.GetSensorReadingFactorsRequest:
		// Factors apply to the exact raw reading 25: M=2, R exponent -1.
		if q.Reading != 25 {
			return nil, fmt.Errorf("wrong factor input %d", q.Reading)
		}
		return []byte{0, 2, 0, 0, 0, 0, 0xf0}, nil
	case *storage.GetSELInfoRequest:
		if f.selErr != nil {
			return nil, f.selErr
		}
		data := make([]byte, 14)
		binary.LittleEndian.PutUint16(data[1:], fakeSELEntries)
		return data, nil
	default:
		return nil, fmt.Errorf("unexpected command %T", req)
	}
}

func (f *fakeBMC) respondSDR(q *storage.GetSDRRequest) ([]byte, error) {
	index := max(int(q.RecordID), 1) - 1 // record 0000 is the first record
	if index >= len(f.records) {
		return nil, fmt.Errorf("bad fake record %d", index)
	}
	// Get SDR: ReadBytes FFh reads the entire record; next record ID FFFFh ends the chain.
	if f.refuseWholeReads && q.ReadBytes == 0xff {
		return nil, types.NewResponseError(types.CodeCannotReturnRequestedDataBytes, "partial required")
	}

	next := uint16(index + 2)
	if index == len(f.records)-1 {
		next = 0xffff
	}
	record := f.records[index]
	if q.ReadBytes != 0xff {
		// Return only bytes the record has, like a BMC reading past its end.
		start := min(int(q.ReadOffset), len(record))
		record = record[start:min(start+int(q.ReadBytes), len(record))]
		if f.partialLength != nil {
			record = record[:min(len(record), f.partialLength(int(q.ReadOffset), int(q.ReadBytes)))]
		}
	} else if f.wholeLength != nil {
		record = record[:min(len(record), f.wholeLength(len(record)))]
	}
	data := make([]byte, 2+len(record))
	binary.LittleEndian.PutUint16(data, next)
	copy(data[2:], record)
	return data, nil
}

// Byte offsets within full and compact sensor records.
const (
	recordOwnerID         = 5
	recordOwnerLUN        = 6 // channel in bits 7:4, LUN in bits 1:0
	fullLinearization     = 23
	fullExponents         = 29 // R exponent in bits 7:4, B exponent in bits 3:0
	compactShareCount     = 23 // modifier type in bits 5:4, count in bits 3:0
	compactModifierOffset = 24
)

// fullRecord encodes a BMC-owned threshold temperature sensor in degrees C with M=1.
func fullRecord(id uint16, number uint8, name string) []byte {
	data := make([]byte, 48+len(name))
	binary.LittleEndian.PutUint16(data, id)
	data[2] = 0x51 // SDR version
	data[3] = byte(types.SDRRecordTypeFullSensor)
	data[4] = byte(len(data) - types.SDRRecordHeaderSize)
	data[recordOwnerID] = types.BMC_SA
	data[7] = number
	data[12] = 0x01 // sensor type: temperature
	data[13] = 0x01 // event/reading type: threshold
	data[20] = 0x00 // unsigned analog, no rate or modifier
	data[21] = byte(types.SensorUnitType_DegreesC)
	data[24] = 1                      // M
	data[47] = 0xc0 | byte(len(name)) // 8-bit ASCII + Latin-1 ID string
	copy(data[48:], name)
	return data
}

// compactRecord encodes a BMC-owned sensor-specific discrete sensor.
func compactRecord(id uint16, number uint8, sensorType uint8, name string) []byte {
	data := make([]byte, 32+len(name))
	binary.LittleEndian.PutUint16(data, id)
	data[2] = 0x51 // SDR version
	data[3] = byte(types.SDRRecordTypeCompactSensor)
	data[4] = byte(len(data) - types.SDRRecordHeaderSize)
	data[recordOwnerID] = types.BMC_SA
	data[7] = number
	data[12] = sensorType
	data[13] = 0x6f                   // event/reading type: sensor-specific
	data[20] = 0xc0                   // no analog reading
	data[31] = 0xc0 | byte(len(name)) // 8-bit ASCII + Latin-1 ID string
	copy(data[32:], name)
	return data
}

func (f *fakeBMC) sensorReadingRequests() []uint8 {
	var numbers []uint8
	for _, req := range f.requests {
		if q, ok := req.(*sensor.GetSensorReadingRequest); ok {
			numbers = append(numbers, q.SensorNumber)
		}
	}
	return numbers
}

func (f *fakeBMC) sdrRequests() []*storage.GetSDRRequest {
	var out []*storage.GetSDRRequest
	for _, req := range f.requests {
		if q, ok := req.(*storage.GetSDRRequest); ok {
			out = append(out, q)
		}
	}
	return out
}
