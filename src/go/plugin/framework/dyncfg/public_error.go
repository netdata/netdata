// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"strings"
)

const publicErrorFallback = "dynamic configuration operation failed"

// PublicError separates a response-safe explanation from its private cause.
// It is the one marker of text that stays visible when the framework redacts
// a job whose configuration resolved secret references.
//
// The public message MUST be code-authored text. It MAY be formatted with
// values from closed protocol domains: numeric status or error codes and the
// code-defined names of those codes. It MUST NOT contain any configuration
// value, because any option (host, URL, user name, path) may be a resolved
// secret reference; nor submitted values, endpoints, credentials, principal or
// account names, server-supplied or backend text, raw library errors, or
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

// PublicMessage returns an explicitly response-safe explanation from err: the
// message of the first PublicError in its chain.
func PublicMessage(err error) (string, bool) {
	var public *PublicError
	if !errors.As(err, &public) || public == nil {
		return "", false
	}
	return public.Error(), true
}
