// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/stretchr/testify/assert"
)

func TestCollectorReceiverReadErrorLogIsRateLimited(t *testing.T) {
	var buf bytes.Buffer
	c := newTestSNMPTrapsCollector()
	c.Logger = logger.NewWithWriter(&buf)
	metrics := newTestJobTelemetry(t, "read-errors", false)
	event := receiver.Event{
		Type: receiver.EventListenerReadFailed,
		Endpoint: receiver.Endpoint{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     9162,
		},
		Err: errors.New("boom"),
	}

	c.handleReceiverEvent(metrics, event)
	event.Err = errors.New("again")
	c.handleReceiverEvent(metrics, event)

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "SNMP trap listener read failed"))
	assert.Contains(t, out, "endpoint=udp4://127.0.0.1:9162")
	assert.Contains(t, out, "boom")
	assert.NotContains(t, out, "again")
	assertJobMetric(t, metrics, "read-errors", "snmp_trap_errors_listener_read_failed", 2)
}

func TestCollectorAttachesTelemetryBeforeHandlingBindEvents(t *testing.T) {
	c := newTestSNMPTrapsCollector()
	c.Name = "buffer-degraded"
	var buf bytes.Buffer
	c.Logger = logger.NewWithWriter(&buf)

	metrics := c.attachTelemetryAndHandleInitEvents([]receiver.Event{{
		Type: receiver.EventListenerBufferDegraded,
		Endpoint: receiver.Endpoint{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     9162,
		},
		Requested: receiver.DefaultReceiveBuffer,
		Err:       errors.New("boom"),
	}}, false)
	t.Cleanup(metrics.Detach)

	assertJobMetric(t, metrics, "buffer-degraded", "snmp_trap_errors_listener_buffer_degraded", 1)
	out := buf.String()
	assert.Contains(t, out, "SNMP trap listener receive buffer request degraded")
	assert.Contains(t, out, "endpoint=udp4://127.0.0.1:9162")
	assert.Contains(t, out, "requested=4194304 bytes")
	assert.Contains(t, out, "boom")
}

func TestCollectorMapsReceiverErrorKinds(t *testing.T) {
	tests := map[receiver.ErrorKind]string{
		receiver.ErrorMalformedPDU:  "snmp_trap_errors_malformed_pdu",
		receiver.ErrorAuthFailure:   "snmp_trap_errors_auth_failures",
		receiver.ErrorUSMFailure:    "snmp_trap_errors_usm_failures",
		receiver.ErrorUnknownEngine: "snmp_trap_errors_unknown_engine_id",
		receiver.ErrorDecodeFailed:  "snmp_trap_errors_decode_failed",
		receiver.ErrorDroppedPolicy: "snmp_trap_errors_dropped_allowlist",
		receiver.ErrorRateLimited:   "snmp_trap_errors_rate_limited",
	}

	for kind, metric := range tests {
		t.Run(string(kind), func(t *testing.T) {
			metrics := newTestJobTelemetry(t, string(kind), false)
			newTestSNMPTrapsCollector().handleReceiverEvent(metrics, receiver.Event{
				Type:      receiver.EventError,
				ErrorKind: kind,
			})
			assertJobMetric(t, metrics, string(kind), metric, 1)
		})
	}
}

func TestCollectorMapsReceiverOperationalEvents(t *testing.T) {
	tests := map[string]struct {
		event     receiver.Event
		metric    string
		logSubstr []string
	}{
		"dynamic engine ID registration": {
			event: receiver.Event{
				Type:     receiver.EventDynamicEngineIDRegistered,
				EngineID: "8000000001020304",
				Username: "test-user",
			},
			metric: "snmp_trap_errors_unknown_engine_id",
			logSubstr: []string{
				"Dynamic SNMPv3 engine ID registered",
				"engineID=8000000001020304",
				"username=test-user",
			},
		},
		"INFORM response failure": {
			event: receiver.Event{
				Type: receiver.EventInformResponseFailed,
				Err:  errors.New("response failed"),
			},
			metric:    "snmp_trap_errors_inform_response_failed",
			logSubstr: []string{"SNMP trap INFORM response failed", "response failed"},
		},
		"discovery Report failure": {
			event: receiver.Event{
				Type: receiver.EventDiscoveryReportFailed,
				Err:  errors.New("report failed"),
			},
			metric:    "snmp_trap_errors_inform_response_failed",
			logSubstr: []string{"SNMP trap INFORM discovery Report failed", "report failed"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metrics := newTestJobTelemetry(t, name, false)
			var buf bytes.Buffer
			c := newTestSNMPTrapsCollector()
			c.Logger = logger.NewWithWriter(&buf)

			c.handleReceiverEvent(metrics, tc.event)

			assertJobMetric(t, metrics, name, tc.metric, 1)
			for _, substr := range tc.logSubstr {
				assert.Contains(t, buf.String(), substr)
			}
		})
	}
}
