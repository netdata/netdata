// SPDX-License-Identifier: GPL-3.0-or-later

package gotify

import (
	"io"
	"net/http"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
)

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "gotify", func(cfg Config) error { _, err := New(cfg, http.DefaultClient); return err })
}
