// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !linux || !cgo || !netdata_ebpf_libbpf

package cachestat

import (
	"context"
	"fmt"
)

func probeNative(context.Context) (string, error) {
	return "", fmt.Errorf("requires Linux, CGO and the netdata_ebpf_libbpf build tag")
}

func openNative(context.Context, string) (nativeRuntime, error) {
	_, err := probeNative(context.Background())
	return nil, err
}
