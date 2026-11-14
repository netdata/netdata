// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !linux

package runtime

import (
	"errors"
	"os"
)

var errNativePlatform = errors.New("native browser runtime requires Linux amd64 or arm64")

func validateNativeConfig(NativeConfig) error                      { return errNativePlatform }
func nativeRootIdentity() error                                    { return errNativePlatform }
func nativeServiceIdentity(nativeControlRecord) error              { return errNativePlatform }
func nativeProtectedPath(string) error                             { return errNativePlatform }
func nativeProtectedFile(*os.Root, string, bool) error             { return errNativePlatform }
func nativeProofDirectory(NativeConfig, nativeControlRecord) error { return errNativePlatform }
func createNativeControl(NativeConfig, int, int) error             { return errNativePlatform }
func readNativePayload(string) (Paths, Manifest, error) {
	return Paths{}, Manifest{}, errNativePlatform
}
