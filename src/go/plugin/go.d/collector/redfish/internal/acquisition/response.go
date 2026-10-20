// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

type wireStats struct {
	started      int
	redirected   int
	received     int64
	successful   int
	failed       int
	failures     map[string]int
	unauthorized bool
}

func (s *wireStats) merge(other *wireStats) {
	if s == nil || other == nil {
		return
	}
	s.started += other.started
	s.redirected += other.redirected
	s.received += other.received
	s.successful += other.successful
	s.failed += other.failed
	if s.failures == nil {
		s.failures = make(map[string]int)
	}
	for class, count := range other.failures {
		s.failures[class] += count
	}
	s.unauthorized = s.unauthorized || other.unauthorized
}

type responseData struct {
	status     int
	body       []byte
	url        *url.URL
	startedAt  time.Time
	finishedAt time.Time
	stats      *wireStats
	finished   bool
}

func (r *responseData) finish(err error) {
	if r == nil || r.finished || r.stats == nil {
		return
	}
	r.finished = true
	if err == nil {
		r.stats.successful++
		return
	}
	r.stats.failed++
	if r.stats.failures == nil {
		r.stats.failures = make(map[string]int)
	}
	r.stats.failures[classifyError(err)]++
}

func decodeJSON(response *responseData, target any) error {
	if response == nil {
		return errors.New("nil Redfish response")
	}
	return decodeJSONBytes(response.body, target)
}

// Preserve exact counter tokens and decode properties locally: SDK resource
// structs use platform-width ints and can reject an entire resource when an
// optional property is malformed.
func decodeJSONBytes(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("Redfish response contains trailing JSON data")
	}
	return nil
}

func metadataForResponse(response *responseData) measurement.ResponseTiming {
	if response == nil {
		return measurement.ResponseTiming{}
	}
	return measurement.ResponseTiming{
		StartedAt:  response.startedAt,
		FinishedAt: response.finishedAt,
	}
}

func linkAt(data map[string]any, path string) string {
	value, _ := measurement.Properties(data).Lookup(path)
	link, _ := value.(map[string]any)
	uri, _ := measurement.Properties(link).Text("@odata.id")
	return uri
}

func stringAt(data map[string]any, path string) string {
	value, _ := measurement.Properties(data).String(path)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func mergeResponseMetadata(left, right measurement.ResponseTiming) measurement.ResponseTiming {
	if left.StartedAt.IsZero() {
		return right
	}
	if right.StartedAt.IsZero() {
		return left
	}
	if right.StartedAt.Before(left.StartedAt) {
		left.StartedAt = right.StartedAt
	}
	if right.FinishedAt.After(left.FinishedAt) {
		left.FinishedAt = right.FinishedAt
	}
	return left
}
