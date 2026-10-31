// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !linux

package main

import (
	"context"
	"errors"
	"io"
)

var errNativeCommandPlatform = errors.New("native runtime package commands require Linux")

func nativeCommandPlatform() error { return errNativeCommandPlatform }

type nativeIdentity struct{ uid, gid uint32 }
type nativeChild struct{}

func nativeServiceAccount() (nativeIdentity, error) {
	return nativeIdentity{}, errNativeCommandPlatform
}
func newNativeChild(nativeIdentity, io.Writer) (*nativeChild, error) {
	return nil, errNativeCommandPlatform
}
func (*nativeChild) run(context.Context, string, []byte) ([]byte, error) {
	return nil, errNativeCommandPlatform
}
