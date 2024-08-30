// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
)

const precision = 1000

func (c *ClickHouse) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectSystemEvents(mx); err != nil {
		return nil, err
	}
	if err := c.collectSystemMetrics(mx); err != nil {
		return nil, err
	}
	if err := c.collectSystemAsyncMetrics(mx); err != nil {
		return nil, err
	}
	if err := c.collectSystemParts(mx); err != nil {
		return nil, err
	}
	if err := c.collectSystemDisks(mx); err != nil {
		return nil, err
	}
	if err := c.collectLongestRunningQueryTime(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *ClickHouse) doOKDecodeCSV(req *http.Request, assign func(column, value string, lineEnd bool)) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	return readCSVResponseData(resp.Body, assign)
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

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
