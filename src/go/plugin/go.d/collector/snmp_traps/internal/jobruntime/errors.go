// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import "errors"

type ErrorKind uint8

const (
	ErrorConfig ErrorKind = iota + 1
	ErrorStartup
)

type Error struct {
	kind ErrorKind
	err  error
}

func (e *Error) Error() string { return e.err.Error() }
func (e *Error) Unwrap() error { return e.err }
func (e *Error) Kind() ErrorKind {
	return e.kind
}

func configError(err error) error  { return &Error{kind: ErrorConfig, err: err} }
func startupError(err error) error { return &Error{kind: ErrorStartup, err: err} }

func KindOf(err error) ErrorKind {
	var target *Error
	if errors.As(err, &target) {
		return target.kind
	}
	return 0
}
