// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"

	"context"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const precision = 1000

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectSystemEvents(ctx, mx); err != nil {
		return nil, err
	}
	if err := c.collectSystemMetrics(ctx, mx); err != nil {
		return nil, err
	}
	if err := c.collectSystemAsyncMetrics(ctx, mx); err != nil {
		return nil, err
	}
	if err := c.collectSystemParts(ctx, mx); err != nil {
		return nil, err
	}
	if err := c.collectSystemDisks(ctx, mx); err != nil {
		return nil, err
	}
	if err := c.collectLongestRunningQueryTime(ctx, mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) doHTTP(req *http.Request, assign func(column, value string, lineEnd bool)) error {
	return web.DoHTTP(c.httpClient).Request(req, func(body io.Reader) error {
		return readCSVResponseData(body, assign)
	})
}

func readCSVResponseData(reader io.Reader, assign func(column, value string, lineEnd bool)) error {
	r := csv.NewReader(reader)
	r.ReuseRecord = true

	var columns []string

	for {
		record, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if len(columns) == 0 {
			columns = slices.Clone(record)
			continue
		}

		if len(columns) != len(record) {
			return fmt.Errorf("column count mismatch: %d vs %d", len(columns), len(record))
		}

		for i, l := 0, len(record); i < l; i++ {
			assign(columns[i], record[i], i == l-1)
		}
	}

	return nil
}

func makeURLQuery(q string) string {
	return url.Values{"query": {q}}.Encode()
}
