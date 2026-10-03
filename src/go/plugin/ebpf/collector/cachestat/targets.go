// SPDX-License-Identifier: GPL-3.0-or-later

package cachestat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

func resolveTargets(ctx context.Context, r io.Reader) (string, error) {
	found := make(map[string]bool)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		f := strings.Fields(scanner.Text())
		if len(f) < 3 || !strings.Contains("TtWw", f[1]) || len(f[1]) != 1 {
			continue
		}
		switch f[2] {
		case "mark_page_accessed", "mark_buffer_dirty", "add_to_page_cache_lru",
			"account_page_dirtied", "__set_page_dirty", "__folio_mark_dirty":
			found[f[2]] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	for _, target := range []string{"mark_page_accessed", "mark_buffer_dirty", "add_to_page_cache_lru"} {
		if !found[target] {
			return "", fmt.Errorf("kernel symbol %s unavailable", target)
		}
	}
	// Keep the existing cachestat backend's candidate preference.
	for _, target := range []string{"account_page_dirtied", "__set_page_dirty", "__folio_mark_dirty"} {
		if found[target] {
			return target, nil
		}
	}
	return "", fmt.Errorf("no supported account-page-dirtied kernel symbol")
}
