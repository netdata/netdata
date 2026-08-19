// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"strings"
)

const publicErrorFallback = "dynamic configuration operation failed"

// PublicError separates a response-safe explanation from its private cause.
// The public message MUST be static, code-authored text. It MUST NOT contain
// submitted or resolved values, endpoints, credentials, backend errors, or
// response bodies.
type PublicError struct {
	message string
	cause   error
}

// NewPublicError marks message as safe for Function responses and Agent logs.
// The cause remains available to internal errors.Is and errors.As checks.
func NewPublicError(message string, cause error) error {
	if cause == nil {
		return nil
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = publicErrorFallback
	}
	return &PublicError{message: message, cause: cause}
}

func (err *PublicError) Error() string {
	if err == nil || err.message == "" {
		return publicErrorFallback
	}
	return err.message
}

func (err *PublicError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

// PublicMessage returns an explicitly response-safe explanation from err.
func PublicMessage(err error) (string, bool) {
	var public *PublicError
	if !errors.As(err, &public) || public == nil {
		return "", false
	}
	return public.Error(), true
}
