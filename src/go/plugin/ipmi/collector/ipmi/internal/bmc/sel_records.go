// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/bougou/go-ipmi/pkg/types"
)

// SEL record layout and timestamp ranges (IPMI 2.0 sections 32 and 37).
const (
	selRecordSize = 16
	// selTimestampedOEMFirst and selNonTimestampedOEMFirst start the OEM record type ranges.
	selTimestampedOEMFirst    = 0xc0
	selNonTimestampedOEMFirst = 0xe0
	// selPreInitTimestampLimit: lower timestamps count seconds since BMC initialization.
	selPreInitTimestampLimit = 0x20000000
	selUnspecifiedTimestamp  = 0xffffffff
	// Event data 1 bits 7:6 and 5:4 mark event data 2 and 3 as the threshold
	// event's trigger reading and trigger threshold.
	selTriggerReadingMask   = 0xc0
	selTriggerReading       = 0x40
	selTriggerThresholdMask = 0x30
	selTriggerThreshold     = 0x10
)

// System software ID ranges of generator IDs with bit 0 set (IPMI 2.0 table 5-4).
const (
	softwareBIOSLast          = 0x1f
	softwareSMIHandlerLast    = 0x3f
	softwareSystemMgmtLast    = 0x5f
	softwareOEMLast           = 0x7f
	softwareRemoteConsoleLast = 0x8d
	softwareTerminalMode      = 0x8f
)

// SELRecord is one System Event Log record, decoded when it was read.
type SELRecord struct {
	ID uint16
	// Timestamp is the raw BMC timestamp; SEL.Time converts it.
	Timestamp   uint32
	Timestamped bool
	// Sensor is the SDR sensor name, or the generator and sensor number when
	// the SDR has no such sensor. Empty for OEM records.
	Sensor     string
	SensorType string
	// Generator names the event generator class and its ID. Empty for OEM records.
	Generator string
	Event     string
	// Direction is Assertion or Deassertion; empty for records without a sensor event.
	Direction string
	State     string
	// Threshold marks threshold events (event/reading type 01h), which also
	// change the state of their sensor.
	Threshold bool
	// Raw is the record as returned by the BMC.
	Raw [selRecordSize]byte
}

// SEL is the System Event Log as cached by the Reader.
type SEL struct {
	// Records are in log order, oldest first. The slice is shared between
	// snapshots and never modified.
	Records []SELRecord
	// Loaded is false until a read of the whole log was verified once; Records
	// are then the oldest part of the log.
	Loaded bool
	// Entries, FreeBytes and Overflow are from the latest Get SEL Info response.
	Entries   int
	FreeBytes int
	Overflow  bool
	// UTCOffset is the BMC's SEL time minus UTC; nil when the BMC does not report it.
	UTCOffset *time.Duration
	// ClockSkew is the BMC SEL clock minus the Agent clock, before applying
	// UTCOffset; nil before the first measurement.
	ClockSkew *time.Duration
	// ReadFailed reports that this collection's Get SEL Info or record reads
	// failed; the other fields are from earlier collections.
	ReadFailed bool
	// New counts newly observed record occupants since the previous eligible
	// snapshot. Initial loading is history; subsequent rescans do not delay
	// event counts. Rereading the same occupant does not count it again.
	New SELCounts
	// NewKnown is false before the first verified baseline and when ReadFailed
	// is set. Pending counts are reported by the next eligible snapshot.
	NewKnown bool
}

// Time returns the UTC time a record was logged. It reports false for records
// without a date: non-timestamped OEM records, pre-init timestamps (seconds
// since BMC initialization) and the unspecified timestamp.
func (s *SEL) Time(r SELRecord) (time.Time, bool) {
	if !r.Timestamped || r.Timestamp < selPreInitTimestampLimit || r.Timestamp == selUnspecifiedTimestamp {
		return time.Time{}, false
	}
	t := time.Unix(int64(r.Timestamp), 0).UTC()
	if s.UTCOffset != nil {
		t = t.Add(-*s.UTCOffset)
	}
	return t, true
}

// BMCTime describes the record's raw timestamp: the BMC clock reading, the
// seconds since BMC initialization, or why there is none.
func (r SELRecord) BMCTime() string {
	switch {
	case !r.Timestamped:
		return "Not timestamped"
	case r.Timestamp == selUnspecifiedTimestamp:
		return "Unspecified"
	case r.Timestamp < selPreInitTimestampLimit:
		return fmt.Sprintf("%d s after BMC initialization", r.Timestamp)
	default:
		return time.Unix(int64(r.Timestamp), 0).UTC().Format(time.DateTime) + " (BMC clock)"
	}
}

// sensorAddress identifies a sensor as a SEL event generator addresses it.
type sensorAddress struct {
	owner  uint8 // slave address or system software ID, bit 0 included
	lun    uint8
	number uint8
}

// sensorsByAddress indexes the inventory for naming SEL records. The first
// record wins when an SDR repository repeats an address.
func sensorsByAddress(inventory []descriptor) map[sensorAddress]*descriptor {
	byAddress := make(map[sensorAddress]*descriptor, len(inventory))
	for i := range inventory {
		d := &inventory[i]
		address := sensorAddress{
			owner:  d.owner.OwnerID(),
			lun:    d.owner.LUN(),
			number: d.number,
		}
		if _, ok := byAddress[address]; !ok {
			byAddress[address] = d
		}
	}
	return byAddress
}

// decodeSELRecord decodes a 16-byte record. System event records get the
// FreeIPMI default state; other records are unknown. Some BMCs log events with
// a reserved record type in the standard range (00h-BFh) but the system event
// layout; they are decoded for display, keep the unknown state, and name their
// record type in the event text.
func decodeSELRecord(raw [selRecordSize]byte, sensors map[sensorAddress]*descriptor) SELRecord {
	r := SELRecord{
		ID:          binary.LittleEndian.Uint16(raw[0:]),
		Timestamp:   binary.LittleEndian.Uint32(raw[3:]),
		Timestamped: raw[2] < selNonTimestampedOEMFirst,
		State:       StateUnknown,
		Raw:         raw,
	}
	recordType := raw[2]
	switch {
	case recordType >= selNonTimestampedOEMFirst:
		r.Event = fmt.Sprintf("OEM record type %#02x", recordType)
		return r
	case recordType >= selTimestampedOEMFirst:
		manufacturer := uint32(raw[7]) | uint32(raw[8])<<8 | uint32(raw[9])<<16
		r.Event = fmt.Sprintf("OEM record type %#02x, manufacturer ID %d", recordType, manufacturer)
		return r
	}

	sel, err := types.ParseSEL(raw[:])
	if err != nil || sel.Standard == nil {
		r.Event = "Undecodable system event record"
		return r
	}
	s := sel.Standard
	eventType, sensorType := uint8(s.EventReadingType), uint8(s.SensorType)
	r.Threshold, r.State = selRecordClass(raw)
	r.Direction = s.EventDir.String()
	r.Generator = generatorName(s.GeneratorID)

	d := sensors[sensorAddress{
		owner:  s.GeneratorID.OwnerID(),
		lun:    s.GeneratorID.LUN(),
		number: uint8(s.SensorNumber),
	}]
	name := fmt.Sprintf("%s #%d", generatorClass(s.GeneratorID), s.SensorNumber)
	if d != nil {
		name = d.sensor.Name
	}
	r.Sensor = name
	r.SensorType, _ = sensorTypeAndComponent(sensorType, name)

	r.Event = s.EventString()
	if r.Event == "" {
		r.Event = fmt.Sprintf("Unrecognized event: type %#02x, offset %d",
			eventType, s.EventData.EventData1&selEventOffsetMask)
	}
	if eventType == eventTypeThreshold && d != nil {
		r.Event += thresholdTrigger(s.EventData, d)
	}
	if recordType != selRecordTypeSystemEvent {
		r.Event += fmt.Sprintf(" [record type %#02x]", recordType)
	}
	return r
}

// selRecordClass reads the event fields from the IPMI 2.0 section 32.1 wire
// layout. Classification is independent of SDR inventory and display decoding.
func selRecordClass(raw [selRecordSize]byte) (threshold bool, state string) {
	if raw[2] >= selTimestampedOEMFirst {
		return false, StateUnknown
	}
	eventType := raw[12] & 0x7f
	return eventType == eventTypeThreshold, selState(raw[2], eventType, raw[10], raw[13], raw[12]&0x80 != 0)
}

// thresholdTrigger formats the trigger reading and threshold that a threshold
// event carries, converted like readings of its sensor. Values that cannot be
// converted are left out.
func thresholdTrigger(data types.EventData, d *descriptor) string {
	convert := func(raw uint8) (string, bool) {
		// Non-linear sensors need conversion factors read for each value.
		if d.sensor.Unit == "" || d.linearization == types.LinearizationFunc_NonLinear {
			return "", false
		}
		v, ok := convertReading(raw, d.unit, d.factors, d.linearization)
		if !ok {
			return "", false
		}
		return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) + " " + d.sensor.Unit, true
	}
	var text string
	if data.EventData1&selTriggerReadingMask == selTriggerReading {
		if v, ok := convert(data.EventData2); ok {
			text = "reading " + v
		}
	}
	if data.EventData1&selTriggerThresholdMask == selTriggerThreshold {
		if v, ok := convert(data.EventData3); ok {
			if text != "" {
				text += ", "
			}
			text += "threshold " + v
		}
	}
	if text == "" {
		return ""
	}
	return " (" + text + ")"
}

// generatorClass names the generator of an event: the BMC, another IPMB
// controller, or a class of system software.
func generatorClass(g types.GeneratorID) string {
	id := g.OwnerID()
	if !g.OwnerIsSystemSoftware() {
		if id == types.BMC_SA {
			return "BMC"
		}
		return fmt.Sprintf("Controller %#02x", id)
	}
	switch {
	case id <= softwareBIOSLast:
		return "BIOS"
	case id <= softwareSMIHandlerLast:
		return "SMI Handler"
	case id <= softwareSystemMgmtLast:
		return "System Management Software"
	case id <= softwareOEMLast:
		return "OEM Software"
	case id <= softwareRemoteConsoleLast:
		return "Remote Console"
	case id == softwareTerminalMode:
		return "Terminal Mode Remote Console"
	default:
		return "Software"
	}
}

// generatorName is the generator class with its raw ID.
func generatorName(g types.GeneratorID) string {
	if !g.OwnerIsSystemSoftware() && g.OwnerID() != types.BMC_SA {
		return generatorClass(g)
	}
	return fmt.Sprintf("%s (%#02x)", generatorClass(g), g.OwnerID())
}
