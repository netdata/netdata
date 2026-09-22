// SPDX-License-Identifier: GPL-3.0-or-later
package smseagle

import (
	"io"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
)

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "smseagle", func(cfg Config) error { _, err := New(cfg, nil); return err })
}

type testBody struct {
	io.Reader
	closed bool
}

func (b *testBody) Close() error { b.closed = true; return nil }

type formErrorReader struct{ err error }

func (r formErrorReader) Read([]byte) (int, error) { return 0, r.err }
