// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
)

const syntheticResolvedSecret = "synthetic-resolved-secret"

func TestRedactResolvedLifecycleErrorPublicMessages(t *testing.T) {
	const publicText = "transport returned HTTP 401"
	public := func(cause error) error { return dyncfg.NewPublicError(publicText, cause) }
	secretCause := errors.New("dial " + syntheticResolvedSecret)
	deadline := time.Now().Add(time.Minute).Truncate(time.Second)
	generic := errResolvedLifecycleRedacted.Error()

	tests := map[string]struct {
		err  error
		want redactionSummary
	}{
		"unmarked error stays generic": {
			err:  errors.New("login failed for " + syntheticResolvedSecret),
			want: redactionSummary{Text: generic},
		},
		"bare public message": {
			err:  public(secretCause),
			want: redactionSummary{Text: publicText, Public: publicText},
		},
		"wrapping text around a public message is dropped": {
			err:  fmt.Errorf("connect %s: %w", syntheticResolvedSecret, public(secretCause)),
			want: redactionSummary{Text: publicText, Public: publicText},
		},
		"joined unmarked text is dropped": {
			err:  errors.Join(errors.New(syntheticResolvedSecret), public(secretCause)),
			want: redactionSummary{Text: publicText, Public: publicText},
		},
		"first public message wins": {
			err:  errors.Join(public(secretCause), dyncfg.NewPublicError("second public message", secretCause)),
			want: redactionSummary{Text: publicText, Public: publicText},
		},
		"public value formatted into a panic is not public": {
			err:  fmt.Errorf("panic %v", public(secretCause)),
			want: redactionSummary{Text: generic},
		},
		"permanent classification survives": {
			err:  collectorapi.PermanentError(public(secretCause)),
			want: redactionSummary{Text: publicText, Public: publicText, Class: collectorapi.LifecycleErrorPermanent},
		},
		"temporary classification survives": {
			err:  collectorapi.TemporaryError(public(secretCause)),
			want: redactionSummary{Text: publicText, Public: publicText, Class: collectorapi.LifecycleErrorTemporary},
		},
		"retry deadline survives": {
			err:  collectorapi.WithRetryNotBefore(public(secretCause), deadline),
			want: redactionSummary{Text: publicText, Public: publicText, RetryNotBefore: deadline},
		},
		"cancellation survives": {
			err: public(fmt.Errorf("%s: %w", syntheticResolvedSecret, context.Canceled)),
			want: redactionSummary{
				Text:     publicText + "\n" + context.Canceled.Error(),
				Public:   publicText,
				Canceled: true,
			},
		},
		"deadline survives": {
			err: fmt.Errorf("%w: %w", public(secretCause), context.DeadlineExceeded),
			want: redactionSummary{
				Text:             publicText + "\n" + context.DeadlineExceeded.Error(),
				Public:           publicText,
				DeadlineExceeded: true,
			},
		},
		"activation dependency survives": {
			err: &collectorapi.ActivationDependencyUnavailableError{Kind: "vnode", Name: "edge", Cause: public(secretCause)},
			want: redactionSummary{
				Text:       "activation dependency vnode/edge is unavailable",
				Public:     publicText,
				Dependency: "vnode/edge",
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			redacted := redactResolvedLifecycleError(test.err)

			assert.Equal(t, test.want, summarizeRedaction(redacted))
			assert.ErrorIs(t, redacted, errResolvedLifecycleRedacted)
			assert.NotErrorIs(t, redacted, secretCause)
			for _, text := range errorTreeTexts(redacted) {
				assert.NotContains(t, text, syntheticResolvedSecret)
			}
		})
	}
}

type redactionSummary struct {
	Text             string
	Public           string
	Class            collectorapi.LifecycleErrorClass
	RetryNotBefore   time.Time
	Canceled         bool
	DeadlineExceeded bool
	Dependency       string
}

func summarizeRedaction(err error) redactionSummary {
	public, _ := dyncfg.PublicMessage(err)
	summary := redactionSummary{
		Text:             err.Error(),
		Public:           public,
		Class:            collectorapi.ClassifyLifecycleError(err),
		RetryNotBefore:   collectorapi.RetryNotBefore(err),
		Canceled:         errors.Is(err, context.Canceled),
		DeadlineExceeded: errors.Is(err, context.DeadlineExceeded),
	}
	var dependency *collectorapi.ActivationDependencyUnavailableError
	if errors.As(err, &dependency) {
		summary.Dependency = dependency.Kind + "/" + dependency.Name
	}
	return summary
}

// errorTreeTexts returns the text of every error reachable from err, so a test
// can prove that no retained node carries raw collector text.
func errorTreeTexts(err error) []string {
	if err == nil {
		return nil
	}
	texts := []string{err.Error()}
	switch wrapped := err.(type) {
	case interface{ Unwrap() error }:
		texts = append(texts, errorTreeTexts(wrapped.Unwrap())...)
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			texts = append(texts, errorTreeTexts(child)...)
		}
	}
	return texts
}
