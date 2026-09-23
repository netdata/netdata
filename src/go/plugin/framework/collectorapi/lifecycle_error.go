// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

// ConfigError classifies an Init or Check error as a configuration failure
// that retrying cannot fix, such as an invalid option or an unknown named
// profile. The job fails without autodetection retries, whatever
// autodetection_retry says, and DynCfg reports code 422.
// It returns nil for a nil err.
func ConfigError(err error) error {
	if err == nil {
		return nil
	}
	return &lifecycleError{err: err, code: 422}
}

// TemporaryError classifies an Init or Check error as a failure that may clear
// on its own, such as a port in use. The job keeps the configured
// autodetection_retry, which an unclassified Init error would disable, and
// DynCfg reports code 503.
// It returns nil for a nil err.
func TemporaryError(err error) error {
	if err == nil {
		return nil
	}
	return &lifecycleError{err: err, code: 503, retryable: true}
}

// lifecycleError carries the markers the Job Manager reads through
// dyncfg.CodedError and dyncfg.RetryableError.
type lifecycleError struct {
	err       error
	code      int
	retryable bool
}

func (e *lifecycleError) Error() string         { return e.err.Error() }
func (e *lifecycleError) Unwrap() error         { return e.err }
func (e *lifecycleError) DyncfgCode() int       { return e.code }
func (e *lifecycleError) DyncfgRetryable() bool { return e.retryable }
