// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleErrorSanitizerFailsClosed(t *testing.T) {
	source := errors.New("source failure")
	require.ErrorIs(t, sanitizeLifecycleError(nil, source), source)
	require.ErrorContains(t, sanitizeLifecycleError(func(error) error { return nil }, source), "discarded")
	require.ErrorContains(t, sanitizeLifecycleError(func(error) error { panic("failed") }, source), "panicked")
}

func TestLifecycleSanitizerPreservesRetryDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	source := collectorapi.WithRetryNotBefore(errors.New("synthetic secret"), deadline)
	for name, sanitize := range map[string]func(error) error{
		"replacement": func(error) error { return errors.New("redacted") },
		"discard":     func(error) error { return nil },
		"panic":       func(error) error { panic("untrusted diagnostic") },
	} {
		t.Run(name, func(t *testing.T) {
			safe := sanitizeLifecycleError(sanitize, source)
			require.Equal(t, deadline, collectorapi.RetryNotBefore(safe))
			require.NotContains(t, safe.Error(), "synthetic secret")
			require.False(t, errors.Is(safe, source))
			failure := newRunFailure(source, "error", sanitize)
			require.Equal(t, deadline, collectorapi.RetryNotBefore(failure))
			require.True(t, failure.Retryable())
		})
	}
}

func TestLifecycleLogMessageSanitizerKeepsOnlyPublicOperandMessages(t *testing.T) {
	const (
		secret     = "synthetic-log-secret"
		publicText = "query failed: status 0x80070005"
		redacted   = "lifecycle redacted"
	)
	public := dyncfg.NewPublicError(publicText, errors.New("server text for "+secret))
	tests := map[string]struct {
		log  func(*logger.Logger)
		want string
	}{
		"public error argument": {
			log:  func(l *logger.Logger) { l.Warning(public) },
			want: publicText,
		},
		"format text around a public error": {
			log:  func(l *logger.Logger) { l.Warningf("query against %s failed: %v", secret, public) },
			want: publicText,
		},
		"wrapped public error": {
			log:  func(l *logger.Logger) { l.Limit("key", 1, time.Minute).Error(fmt.Errorf("%s: %w", secret, public)) },
			want: publicText,
		},
		"unmarked line": {
			log:  func(l *logger.Logger) { l.Warningf("login failed for %s", secret) },
			want: redacted,
		},
		"unmarked error argument": {
			log:  func(l *logger.Logger) { l.Warning(errors.New(secret)) },
			want: redacted,
		},
		"public attribute value": {
			log:  func(l *logger.Logger) { l.With("err", public).Info("attribute line") },
			want: "redacted_attribute=\"" + publicText + "\"",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			// The test policy keeps a public message, as the Job Manager redactor does.
			sanitize := LifecycleLogMessageSanitizer(func(err error) error {
				if message, ok := dyncfg.PublicMessage(err); ok {
					return errors.New(message)
				}
				return errors.New(redacted)
			})
			log := logger.NewWithWriter(&buf).WithMessageSanitizer(sanitize)

			test.log(log)

			assert.Contains(t, buf.String(), test.want)
			assert.NotContains(t, buf.String(), secret)
			assert.NotContains(t, buf.String(), "server text")
		})
	}
}

func TestLifecycleLogMessageSanitizerHandsLineToLifecyclePolicy(t *testing.T) {
	public := dyncfg.NewPublicError("public text", errors.New("private cause"))
	var received []error
	sanitize := LifecycleLogMessageSanitizer(func(err error) error {
		received = append(received, err)
		return errors.New("redacted")
	})
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf).WithMessageSanitizer(sanitize)

	log.Warningf("line text: %v", public)
	log.Warning("unmarked line")

	require.Len(t, received, 2)
	message, ok := dyncfg.PublicMessage(received[0])
	assert.True(t, ok)
	assert.Equal(t, "public text", message)
	assert.Equal(t, "line text: public text", errors.Unwrap(received[0]).Error())
	_, ok = dyncfg.PublicMessage(received[1])
	assert.False(t, ok)
	assert.Equal(t, "unmarked line", received[1].Error())
	// A policy that ignores public messages still redacts the whole line.
	assert.Equal(t, 2, strings.Count(buf.String(), "redacted"))
	assert.NotContains(t, buf.String(), "public text")
	assert.Nil(t, LifecycleLogMessageSanitizer(nil))
}
