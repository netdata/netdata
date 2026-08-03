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
	metrics := &perJobMetrics{}
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
	assert.Equal(t, uint64(2), metrics.errors.listenerReadFailed.Load())
}

func TestCollectorReportsReceiverBufferDegradedEvent(t *testing.T) {
	metrics := &perJobMetrics{}
	c := newTestSNMPTrapsCollector()
	var buf bytes.Buffer
	c.Logger = logger.NewWithWriter(&buf)

	c.handleReceiverEvent(metrics, receiver.Event{
		Type: receiver.EventListenerBufferDegraded,
		Endpoint: receiver.Endpoint{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     9162,
		},
		Requested: receiver.DefaultReceiveBuffer,
		Err:       errors.New("boom"),
	})

	assert.Equal(t, uint64(1), metrics.errors.listenerBufferDegraded.Load())
	out := buf.String()
	assert.Contains(t, out, "SNMP trap listener receive buffer request degraded")
	assert.Contains(t, out, "endpoint=udp4://127.0.0.1:9162")
	assert.Contains(t, out, "requested=4194304 bytes")
	assert.Contains(t, out, "boom")
}

func TestCollectorMapsReceiverErrorKinds(t *testing.T) {
	tests := map[receiver.ErrorKind]func(*perJobMetrics) uint64{
		receiver.ErrorMalformedPDU:  func(m *perJobMetrics) uint64 { return m.errors.malformedPDU.Load() },
		receiver.ErrorAuthFailure:   func(m *perJobMetrics) uint64 { return m.errors.authFailures.Load() },
		receiver.ErrorUSMFailure:    func(m *perJobMetrics) uint64 { return m.errors.usmFailures.Load() },
		receiver.ErrorUnknownEngine: func(m *perJobMetrics) uint64 { return m.errors.unknownEngineID.Load() },
		receiver.ErrorDecodeFailed:  func(m *perJobMetrics) uint64 { return m.errors.decodeFailed.Load() },
		receiver.ErrorDroppedPolicy: func(m *perJobMetrics) uint64 { return m.errors.droppedAllowlist.Load() },
		receiver.ErrorRateLimited:   func(m *perJobMetrics) uint64 { return m.errors.rateLimited.Load() },
	}

	for kind, value := range tests {
		t.Run(string(kind), func(t *testing.T) {
			metrics := &perJobMetrics{}
			newTestSNMPTrapsCollector().handleReceiverEvent(metrics, receiver.Event{
				Type:      receiver.EventError,
				ErrorKind: kind,
			})
			assert.Equal(t, uint64(1), value(metrics))
		})
	}
}

func TestCollectorMapsReceiverOperationalEvents(t *testing.T) {
	tests := map[string]struct {
		event     receiver.Event
		metric    func(*perJobMetrics) uint64
		logSubstr []string
	}{
		"dynamic engine ID registration": {
			event: receiver.Event{
				Type:     receiver.EventDynamicEngineIDRegistered,
				EngineID: "8000000001020304",
				Username: "test-user",
			},
			metric: func(m *perJobMetrics) uint64 { return m.errors.unknownEngineID.Load() },
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
			metric:    func(m *perJobMetrics) uint64 { return m.errors.informResponseFail.Load() },
			logSubstr: []string{"SNMP trap INFORM response failed", "response failed"},
		},
		"discovery Report failure": {
			event: receiver.Event{
				Type: receiver.EventDiscoveryReportFailed,
				Err:  errors.New("report failed"),
			},
			metric:    func(m *perJobMetrics) uint64 { return m.errors.informResponseFail.Load() },
			logSubstr: []string{"SNMP trap INFORM discovery Report failed", "report failed"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metrics := &perJobMetrics{}
			var buf bytes.Buffer
			c := newTestSNMPTrapsCollector()
			c.Logger = logger.NewWithWriter(&buf)

			c.handleReceiverEvent(metrics, tc.event)

			assert.Equal(t, uint64(1), tc.metric(metrics))
			for _, substr := range tc.logSubstr {
				assert.Contains(t, buf.String(), substr)
			}
		})
	}
}
