// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

// LifecycleErrorClass is a collector's classification of an Init, Check or
// CollectorV2Runner.Run error. The Job Manager derives retries, stock-job
// listing and DynCfg replies from it.
type LifecycleErrorClass uint8

const (
	// LifecycleErrorUnclassified is an error carrying neither class; the
	// framework defaults apply.
	LifecycleErrorUnclassified LifecycleErrorClass = iota
	// LifecycleErrorPermanent marks an error returned through PermanentError.
	LifecycleErrorPermanent
	// LifecycleErrorTemporary marks an error returned through TemporaryError.
	LifecycleErrorTemporary
)

// PermanentError classifies an Init or Check error as permanent: retrying the
// same configuration cannot succeed, as with an invalid option or an unknown
// named profile. The job fails without autodetection retries, whatever
// autodetection_retry says. DynCfg update preflight rejects it (422), preserving
// the incumbent. Enable accepts intent before probing and reports a later
// failure through job status. A CollectorV2Runner.Run startup error classified
// this way is not retried either.
// It returns nil for a nil err.
func PermanentError(err error) error {
	if err == nil {
		return nil
	}
	return &lifecycleError{err: err, class: LifecycleErrorPermanent}
}

// TemporaryError classifies an Init or Check error as a failure that may clear
// on its own, such as a dependency that is not ready yet. The job keeps the
// configured autodetection_retry, which an unclassified Init error would
// disable. DynCfg update preflight rejects it (503), preserving the incumbent
// regardless of autodetection_retry; test also answers 503. Enable accepts
// intent before probing and reports a later failure through job status.
// It returns nil for a nil err.
func TemporaryError(err error) error {
	if err == nil {
		return nil
	}
	return &lifecycleError{err: err, class: LifecycleErrorTemporary}
}

// ClassifyLifecycleError returns the class of err. A permanent classification
// anywhere in the error tree wins over a temporary one, because retrying
// cannot fix the permanent part.
func ClassifyLifecycleError(err error) LifecycleErrorClass {
	class := LifecycleErrorUnclassified
	walkErrorTree(err, func(err error) bool {
		classified, ok := err.(*lifecycleError)
		if !ok {
			return true
		}
		if classified.class == LifecycleErrorPermanent {
			class = LifecycleErrorPermanent
			return false
		}
		class = classified.class
		return true
	})
	return class
}

// walkErrorTree visits err and its wrapped errors depth-first until visit
// returns false.
func walkErrorTree(err error, visit func(error) bool) bool {
	if err == nil {
		return true
	}
	if !visit(err) {
		return false
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() error }:
		return walkErrorTree(wrapped.Unwrap(), visit)
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			if !walkErrorTree(child, visit) {
				return false
			}
		}
	}
	return true
}

type lifecycleError struct {
	err   error
	class LifecycleErrorClass
}

func (e *lifecycleError) Error() string { return e.err.Error() }
func (e *lifecycleError) Unwrap() error { return e.err }
