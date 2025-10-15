// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"bufio"
	"strings"
)

const queryShowEngineInnoDBStatus = "SHOW ENGINE INNODB STATUS;"

// collect Checkpoint Age in InnoDB with respect to
// https://mariadb.com/kb/en/innodb-redo-log/#determining-the-checkpoint-age
func (c *Collector) collectEngineInnoDBStatus(mx map[string]int64) error {
	q := queryShowEngineInnoDBStatus
	c.Debugf("executing query: '%s'", q)

	_, err := c.collectQuery(q, func(column, value string, _ bool) {
		switch column {
		case "Status":
			scanner := bufio.NewScanner(strings.NewReader(value))

			for scanner.Scan() {
				line := scanner.Text()
				switch {
				case strings.HasPrefix(line, "Log sequence number"):
					value := strings.TrimSpace(strings.TrimPrefix(line, "Log sequence number"))
					mx["innodb_log_sequence_number"] = parseInt(value)
				case strings.HasPrefix(line, "Last checkpoint at"):
					value := strings.TrimSpace(strings.TrimPrefix(line, "Last checkpoint at"))
					mx["innodb_last_checkpoint_at"] = parseInt(value)
				}
			}
		}
	})
	mx["innodb_checkpoint_age"] = mx["innodb_log_sequence_number"] - mx["innodb_last_checkpoint_at"]
	return err
}
