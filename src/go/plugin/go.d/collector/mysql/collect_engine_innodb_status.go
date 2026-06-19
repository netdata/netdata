// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"bufio"
	"context"
	"fmt"
	"strings"
)

const queryShowEngineInnoDBStatus = "SHOW ENGINE INNODB STATUS;"
const innodbStatusScanMaxToken = 1024 * 1024

// collect Checkpoint Age in InnoDB with respect to
// https://mariadb.com/kb/en/innodb-redo-log/#determining-the-checkpoint-age
func (c *Collector) collectEngineInnoDBStatus(ctx context.Context, state *collectRunState) error {
	q := queryShowEngineInnoDBStatus
	c.Debugf("executing query: '%s'", q)

	var logSequenceNumber, lastCheckpointAt int64
	var parseErr error
	_, err := c.collectQuery(ctx, q, func(column, value string, _ bool) {
		switch column {
		case "Status":
			scanner := bufio.NewScanner(strings.NewReader(value))
			scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), innodbStatusScanMaxToken)

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
			if scanErr := scanner.Err(); scanErr != nil {
				parseErr = fmt.Errorf("scan innodb status: %w", scanErr)
			}
		}
	})
	if err != nil {
		return err
	}
	if parseErr != nil {
		return parseErr
	}

	c.mx.set("innodb_log_sequence_number", logSequenceNumber)
	c.mx.set("innodb_last_checkpoint_at", lastCheckpointAt)

	checkpointAge := logSequenceNumber - lastCheckpointAt
	c.mx.set("innodb_checkpoint_age", checkpointAge)
	state.innodbCheckpointAge = checkpointAge

	return nil
}
