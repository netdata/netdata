// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import "fmt"

type wrappedStoreError struct {
	msg string
	err error
}

func (err wrappedStoreError) Error() string { return err.msg }
func (err wrappedStoreError) Unwrap() error { return err.err }

func storeNotConfiguredError(key string) error {
	return wrappedStoreError{
		msg: fmt.Sprintf("store '%s' is not configured", key),
		err: ErrStoreNotFound,
	}
}
