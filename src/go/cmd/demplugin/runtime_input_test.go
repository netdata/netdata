// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeInputUnsupported(t *testing.T) {
	ctx, finish, err := runtimeInputContext(context.Background(), os.Stdin)
	require.Nil(t, ctx)
	require.Nil(t, finish)
	require.ErrorContains(t, err, "require Linux")
}
