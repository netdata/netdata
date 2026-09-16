// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type wireStats struct {
	started    int
	retried    int
	redirected int
	received   int64
	successful int
	failed     int
	failures   map[string]int
	responses  map[string]struct{}
}

func (s *wireStats) merge(other *wireStats) {
	if s == nil || other == nil {
		return
	}
	s.started += other.started
	s.retried += other.retried
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
	if s.responses == nil {
		s.responses = make(map[string]struct{})
	}
	for key := range other.responses {
		s.responses[key] = struct{}{}
	}
}

type responseData struct {
	status     int
	header     http.Header
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
		recordResponseCompatibility(r.stats, r.header)
		return
	}
	r.stats.failed++
	if r.stats.failures == nil {
		r.stats.failures = make(map[string]int)
	}
	r.stats.failures[classifyError(err)]++
}

type responseMetadata struct {
	StartedAt  time.Time
	FinishedAt time.Time
}

func recordResponseCompatibility(stats *wireStats, header http.Header) {
	if stats == nil {
		return
	}
	if stats.responses == nil {
		stats.responses = make(map[string]struct{})
	}
	for name, state := range map[string]string{
		"Content-Type":  responseContentTypeState(header),
		"OData-Version": responseODataVersionState(header),
	} {
		if state == "valid" {
			continue
		}
		stats.responses[name+"\x00"+state] = struct{}{}
	}
}

func responseCompatibilityDiagnostics(stats *wireStats) []string {
	if stats == nil {
		return nil
	}
	var result []string
	for _, name := range []string{"Content-Type", "OData-Version"} {
		for _, state := range []string{"missing", "invalid"} {
			if _, ok := stats.responses[name+"\x00"+state]; !ok {
				continue
			}
			result = append(result, responseCompatibilityDiagnostic(name, state))
		}
	}
	return result
}

func appendUniqueDiagnostics(current []string, values ...string) []string {
	seen := make(map[string]struct{}, len(current)+len(values))
	for _, value := range current {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		current = append(current, value)
	}
	return current
}

func (c *protocolClient) rememberCompatibilityDiagnostics(stats *wireStats) {
	diagnostics := responseCompatibilityDiagnostics(stats)
	if len(diagnostics) == 0 {
		return
	}
	c.diagnosticMu.Lock()
	if c.pendingCompatibilityDiag == nil {
		c.pendingCompatibilityDiag = make(map[string]struct{})
	}
	for _, diagnostic := range diagnostics {
		c.pendingCompatibilityDiag[diagnostic] = struct{}{}
	}
	c.diagnosticMu.Unlock()
}

func (c *protocolClient) takeCompatibilityDiagnostics() []string {
	c.diagnosticMu.Lock()
	defer c.diagnosticMu.Unlock()
	var result []string
	for _, name := range []string{"Content-Type", "OData-Version"} {
		for _, state := range []string{"missing", "invalid"} {
			diagnostic := responseCompatibilityDiagnostic(name, state)
			if _, ok := c.pendingCompatibilityDiag[diagnostic]; ok {
				result = append(result, diagnostic)
			}
		}
	}
	c.pendingCompatibilityDiag = nil
	return result
}

func decodeJSON(response *responseData, target any) error {
	if response == nil {
		return errors.New("nil Redfish response")
	}
	return decodeJSONBytes(response.body, target)
}

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

func metadataForResponse(response *responseData) responseMetadata {
	if response == nil {
		return responseMetadata{}
	}
	return responseMetadata{
		StartedAt:  response.startedAt,
		FinishedAt: response.finishedAt,
	}
}

func responseContentTypeState(header http.Header) string {
	value := header.Get("Content-Type")
	if len(value) > maxContentTypeBytes {
		return "invalid"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "missing"
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return "invalid"
	}
	if charset := strings.TrimSpace(params["charset"]); charset != "" &&
		!strings.EqualFold(charset, "utf-8") {
		return "invalid"
	}
	return "valid"
}

func responseODataVersionState(header http.Header) string {
	value := header.Get("OData-Version")
	if len(value) > maxODataVersionBytes {
		return "invalid"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "missing"
	}
	if value != "4.0" {
		return "invalid"
	}
	return "valid"
}

func stringAt(data map[string]any, path string) string {
	value, _ := stringValueAt(data, path)
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

func responseCompatibilityDiagnostic(name, state string) string {
	return boundedDiagnostic(fmt.Sprintf("Redfish compatibility: response %s header is %s", name, state))
}

const (
	maxContentTypeBytes  = 1024
	maxODataVersionBytes = 64
)

func mergeResponseMetadata(left, right responseMetadata) responseMetadata {
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
