// SPDX-License-Identifier: GPL-3.0-or-later
//go:build !linux

package ndexec

func CreateTreeCompletion(string) (*TreeCompletion, error) {
	return nil, ErrTreeSupervisionUnsupported
}

func ReadTreeCompletion(string, string) (bool, error) {
	return false, ErrTreeSupervisionUnsupported
}
