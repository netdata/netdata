// SPDX-License-Identifier: GPL-3.0-or-later

package dovecot

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// FIXME: drop using "old_stats" in favour of "stats" (https://doc.dovecot.org/configuration_manual/stats/openmetrics/).

func (d *Dovecot) collect() (map[string]int64, error) {
	if d.conn == nil {
		conn, err := d.establishConn()
		if err != nil {
			return nil, err
		}
		d.conn = conn
	}

	stats, err := d.conn.queryExportGlobal()
	if err != nil {
		d.conn.disconnect()
		d.conn = nil
		return nil, err
	}

	mx := make(map[string]int64)

	// https://doc.dovecot.org/configuration_manual/stats/old_statistics/#statistics-gathered
	if err := d.collectExportGlobal(mx, stats); err != nil {
		return nil, err
	}

	return mx, nil
}

func (d *Dovecot) collectExportGlobal(mx map[string]int64, resp []byte) error {
	sc := bufio.NewScanner(bytes.NewReader(resp))

	if !sc.Scan() {
		return errors.New("failed to read fields line from export global response")
	}
	fieldsLine := strings.TrimSpace(sc.Text())

	if !sc.Scan() {
		return errors.New("failed to read values line from export global response")
	}
	valuesLine := strings.TrimSpace(sc.Text())

	if fieldsLine == "" || valuesLine == "" {
		return errors.New("empty fields line or values line from export global response")
	}

	fields := strings.Fields(fieldsLine)
	values := strings.Fields(valuesLine)

	if len(fields) != len(values) {
		return fmt.Errorf("mismatched fields and values count: fields=%d, values=%d", len(fields), len(values))
	}

	for i, name := range fields {
		val := values[i]

		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			d.Debugf("failed to parse export value %s %s: %v", name, val, err)
			continue
		}

		mx[name] = v
	}

	return nil
}

func (d *Dovecot) establishConn() (dovecotConn, error) {
	conn := d.newConn(d.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
