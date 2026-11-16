// SPDX-License-Identifier: GPL-3.0-or-later

package rocketchat

import (
	"io"
	"net/http"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
)

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "rocketchat", func(cfg Config) error { _, err := New(cfg, http.DefaultClient); return err })
}

func configFields(dst Config) map[string]any {
	data, _ := yaml.Marshal(dst)
	fields := map[string]any{}
	_ = yaml.Unmarshal(data, &fields)
	fields["type"] = "rocketchat"
	return fields
}
