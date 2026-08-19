// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"errors"
	"io"
)

var errResolvedValueTooLarge = errors.New("resolved secret exceeds maximum size")

func readBoundedSecret(reader io.Reader, maximum int) ([]byte, error) {
	if reader == nil || maximum <= 0 {
		return nil, errors.New("invalid bounded secret read")
	}
	value, err := io.ReadAll(io.LimitReader(reader, int64(maximum)+1))
	if err != nil {
		return nil, err
	}
	if len(value) > maximum {
		return nil, errResolvedValueTooLarge
	}
	return value, nil
}
