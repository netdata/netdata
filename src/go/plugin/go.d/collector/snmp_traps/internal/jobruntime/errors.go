// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import "errors"

type ErrorKind uint8

const (
	ErrorConfig ErrorKind = iota + 1
	ErrorStartup
)

type codedError struct {
	kind ErrorKind
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

func configError(err error) error  { return &codedError{kind: ErrorConfig, err: err} }
func startupError(err error) error { return &codedError{kind: ErrorStartup, err: err} }

func KindOf(err error) ErrorKind {
	var target *codedError
	if errors.As(err, &target) {
		return target.kind
	}
	return 0
}
