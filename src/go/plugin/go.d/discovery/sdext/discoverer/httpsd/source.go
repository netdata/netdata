// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"encoding/json"
	"fmt"
	"net/url"
)

func sourceString(cfg Config) string {
	src := fmt.Sprintf("discoverer=%s,url=%s,hash=%x", shortName, sanitizedURL(cfg.URL), sourceHash(cfg))
	if cfg.Source != "" {
		src += fmt.Sprintf(",%s", cfg.Source)
	}
	return src
}

func sanitizedURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func sourceHash(cfg Config) uint64 {
	type identity struct {
		HTTPConfig any    `json:"http_config"`
		Format     string `json:"format"`
	}

	bs, _ := json.Marshal(identity{
		HTTPConfig: cfg.HTTPConfig,
		Format:     cfg.format(),
	})
	return hashBytes(bs)
}
