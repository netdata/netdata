// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package main

import (
	"context"
	"errors"
	"os"
)

func runtimeInputContext(context.Context, *os.File) (context.Context, func(), error) {
	return nil, nil, errors.New("runtime stdin control channels require Linux")
}
