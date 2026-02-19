// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"bufio"
	"context"
	"strings"
)

const queryShowEngineInnoDBStatus = "SHOW ENGINE INNODB STATUS;"

// collect Checkpoint Age in InnoDB with respect to
// https://mariadb.com/kb/en/innodb-redo-log/#determining-the-checkpoint-age
func (c *Collector) collectEngineInnoDBStatus(ctx context.Context, state *collectRunState) error {
	q := queryShowEngineInnoDBStatus
	c.Debugf("executing query: '%s'", q)

	var logSequenceNumber, lastCheckpointAt int64
	_, err := c.collectQuery(ctx, q, func(column, value string, _ bool) {
		switch column {
		case "Status":
			scanner := bufio.NewScanner(strings.NewReader(value))

			for scanner.Scan() {
				line := scanner.Text()
				switch {
				case strings.HasPrefix(line, "Log sequence number"):
					value := strings.TrimSpace(strings.TrimPrefix(line, "Log sequence number"))
					logSequenceNumber = parseInt(value)
				case strings.HasPrefix(line, "Last checkpoint at"):
					value := strings.TrimSpace(strings.TrimPrefix(line, "Last checkpoint at"))
					lastCheckpointAt = parseInt(value)
				}
			}
		}
	})

	c.mx.set("innodb_log_sequence_number", logSequenceNumber)
	c.mx.set("innodb_last_checkpoint_at", lastCheckpointAt)

	checkpointAge := logSequenceNumber - lastCheckpointAt
	c.mx.set("innodb_checkpoint_age", checkpointAge)
	state.innodbCheckpointAge = checkpointAge

	return err
}
