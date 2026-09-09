// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import "errors"

type retainedOwnershipError struct {
	cause error
}

func (e *retainedOwnershipError) Error() string {
	return e.cause.Error()
}

func (e *retainedOwnershipError) Unwrap() error {
	return e.cause
}

// RetainOwnership marks err as a failure that retained or obscured ownership.
func RetainOwnership(err error) error {
	if err == nil || OwnershipRetained(err) {
		return err
	}
	return &retainedOwnershipError{
		cause: err,
	}
}

// OwnershipRetained reports whether err contains a retained-ownership failure.
func OwnershipRetained(err error) bool {
	var retained *retainedOwnershipError
	return errors.As(err, &retained)
}
