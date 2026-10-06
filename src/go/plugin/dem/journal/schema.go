// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"fmt"
	"strconv"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

const (
	schemaField     = "DEM_SCHEMA"
	schemaVersion   = "1"
	rumBucket       = "DEM_RUM_MINUTE"
	syntheticBucket = "DEM_SYNTHETIC_MINUTE"
	minuteUS        = int64(60_000_000)
)

// Envelope validates the shared scalar fields while a domain decoder visits
// borrowed payloads. Domain identity and payload validation stay with that decoder.
type Envelope struct {
	seen              uint8
	kind              string
	timestamp, bucket int64
}

// Observe consumes one field without retaining borrowed bytes. Unknown fields
// belong to the domain decoder. Every shared field must occur at most once.
func (e *Envelope) Observe(name, value []byte) error {
	var bit uint8
	switch string(name) {
	case schemaField:
		bit = 1
		if string(value) != schemaVersion {
			return fmt.Errorf("incompatible DEM history schema %q", string(value))
		}
	case "DEM_KIND":
		bit = 2
		switch string(value) {
		case "rum":
			e.kind = "rum"
		case "synthetic":
			e.kind = "synthetic"
		default:
			return fmt.Errorf("invalid DEM_KIND %q", string(value))
		}
	case "DEM_OBSERVED_US":
		bit = 4
	case "DEM_STARTED_US":
		bit = 8
	case rumBucket:
		bit = 16
	case syntheticBucket:
		bit = 32
	default:
		return nil
	}
	if e.seen&bit != 0 {
		return fmt.Errorf("duplicate history field %s", string(name))
	}
	e.seen |= bit
	if bit >= 4 {
		n, err := decimal(value)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", string(name), err)
		}
		if bit < 16 {
			e.timestamp = n
		} else {
			e.bucket = n
		}
	}
	return nil
}

// Validate returns the domain clock only when the complete envelope agrees.
func (e *Envelope) Validate() (kind string, timestamp int64, err error) {
	var required uint8
	switch e.kind {
	case "rum":
		required = 1 | 2 | 4 | 16
	case "synthetic":
		required = 1 | 2 | 8 | 32
	default:
		return "", 0, fmt.Errorf("invalid DEM_KIND %q", e.kind)
	}
	if e.seen != required || e.bucket != e.timestamp/minuteUS || (e.kind == "synthetic" && e.timestamp == 0) {
		return "", 0, fmt.Errorf("invalid %s history clock/schema envelope", e.kind)
	}
	return e.kind, e.timestamp, nil
}

func decimal(value []byte) (int64, error) {
	if len(value) == 0 || (len(value) > 1 && value[0] == '0') {
		return 0, fmt.Errorf("expected canonical nonnegative decimal")
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("expected canonical nonnegative decimal")
		}
	}
	return strconv.ParseInt(string(value), 10, 64)
}

// appendEnvelope derives index coverage at the single append boundary. Callers
// cannot declare their own schema or bucket and accidentally bypass this encoder.
func appendEnvelope(fields []journal.Field) ([]journal.Field, error) {
	var env Envelope
	for _, field := range fields {
		switch field.Name {
		case schemaField, rumBucket, syntheticBucket:
			return nil, fmt.Errorf("history field %s is reserved for the journal envelope", field.Name)
		}
		if err := env.Observe([]byte(field.Name), field.Value); err != nil {
			return nil, err
		}
	}
	var bucket string
	switch env.kind {
	case "rum":
		if env.seen != 2|4 {
			return nil, fmt.Errorf("RUM history requires exactly one observation timestamp")
		}
		bucket = rumBucket
	case "synthetic":
		if env.seen != 2|8 || env.timestamp == 0 {
			return nil, fmt.Errorf("synthetic history requires exactly one positive start timestamp")
		}
		bucket = syntheticBucket
	default:
		return nil, fmt.Errorf("invalid DEM_KIND %q", env.kind)
	}
	return append(fields, journal.StringField(schemaField, schemaVersion), journal.StringField(bucket, strconv.FormatInt(env.timestamp/minuteUS, 10))), nil
}
