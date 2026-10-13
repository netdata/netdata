// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import "io"

type TrackingBody struct {
	io.Reader
	Closed bool
}

func (b *TrackingBody) Close() error { b.Closed = true; return nil }

type ErrorReader struct{ Err error }

func (r ErrorReader) Read([]byte) (int, error) { return 0, r.Err }
