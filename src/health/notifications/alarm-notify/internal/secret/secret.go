// SPDX-License-Identifier: GPL-3.0-or-later

package secret

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// IsReference checks whole-value secret reference syntax without resolving it.
func IsReference(value string) (bool, error) {
	if !strings.Contains(value, "${") {
		return false, nil
	}
	scheme, operand, ok := strings.Cut(value, ":")
	if !ok || !strings.HasSuffix(operand, "}") {
		return false, errors.New("expected a whole env or file secret reference")
	}
	operand = strings.TrimSuffix(operand, "}")
	if strings.TrimSpace(operand) == "" || strings.ContainsAny(operand, "{}") {
		return false, errors.New("secret reference requires a nonempty operand without braces")
	}
	switch scheme {
	case "${env":
		return true, nil
	case "${file":
		if filepath.IsAbs(operand) {
			return true, nil
		}
		return false, errors.New("file secret reference requires an absolute path")
	default:
		return false, errors.New("only whole env and file secret references are supported")
	}
}

// Resolve reads a whole-value reference lazily, or returns a literal unchanged.
func Resolve(ctx context.Context, value string) (string, error) {
	reference, err := IsReference(value)
	if err != nil {
		return "", err
	}
	if !reference {
		return value, nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	scheme, operand, _ := strings.Cut(value, ":")
	operand = strings.TrimSuffix(operand, "}")
	var text string
	if scheme == "${env" {
		var ok bool
		text, ok = os.LookupEnv(operand)
		if !ok {
			return "", errors.New("secret environment variable is not set")
		}
	} else {
		data, err := os.ReadFile(operand)
		if err != nil {
			return "", errors.New("could not read secret file; check its path and access permissions")
		}
		text = string(data)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("secret resolved to an empty value")
	}
	return text, nil
}
