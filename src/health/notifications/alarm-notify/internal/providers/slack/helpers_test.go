// SPDX-License-Identifier: GPL-3.0-or-later

package slack

import (
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
)

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "slack", func(cfg Config) error { _, err := New(cfg, http.DefaultClient); return err })
}

func configForURL(url string) string {
	return fmt.Sprintf("version: 1\ndestinations:\n  dev:\n    type: webhook\n    url: %q\n", url)
}
