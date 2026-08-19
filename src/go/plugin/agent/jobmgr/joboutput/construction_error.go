// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"errors"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type constructionErrorClass uint8

const (
	constructionErrorOperational constructionErrorClass = iota
	constructionErrorProposal
	constructionErrorTransient
)

type invalidJobConfigurationError struct {
	cause error
}

func (err *invalidJobConfigurationError) Error() string {
	return err.cause.Error()
}

func (err *invalidJobConfigurationError) Unwrap() error {
	return err.cause
}

type transientJobConstructionError struct {
	cause error
}

func (err *transientJobConstructionError) Error() string {
	return err.cause.Error()
}

func (err *transientJobConstructionError) Unwrap() error {
	return err.cause
}

func invalidJobConfiguration(err error) error {
	if err == nil {
		return nil
	}
	return &invalidJobConfigurationError{cause: err}
}

func transientJobConstruction(err error) error {
	if err == nil {
		return nil
	}
	return &transientJobConstructionError{cause: err}
}

func classifyConstructionError(err error) constructionErrorClass {
	var transient *transientJobConstructionError
	if errors.As(err, &transient) {
		return constructionErrorTransient
	}

	var resolveErr *secretresolver.AtomicResolveError
	if errors.As(err, &resolveErr) {
		switch resolveErr.Kind {
		case secretresolver.AtomicErrorProvider, secretresolver.AtomicErrorScope:
			return constructionErrorTransient
		default:
			return constructionErrorProposal
		}
	}

	var invalid *invalidJobConfigurationError
	if errors.As(err, &invalid) {
		return constructionErrorProposal
	}
	return constructionErrorOperational
}

func transientActivationFailure(config confgroup.Config, err error) *autoDetectionFailure {
	return &autoDetectionFailure{
		cause:      err,
		retry:      true,
		retryAfter: config.AutoDetectionRetry(),
	}
}
