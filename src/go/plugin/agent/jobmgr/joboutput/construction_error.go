// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import "github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

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
	return &invalidJobConfigurationError{
		cause: err,
	}
}

func transientJobConstruction(err error) error {
	if err == nil {
		return nil
	}
	return &transientJobConstructionError{
		cause: err,
	}
}

func transientActivationFailure(config confgroup.Config, err error) *autoDetectionFailure {
	return &autoDetectionFailure{
		cause:      err,
		retry:      true,
		retryAfter: config.AutoDetectionRetry(),
	}
}
