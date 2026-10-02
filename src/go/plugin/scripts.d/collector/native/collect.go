// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"fmt"
)

func (c *Collector) collectSnapshot(ctx context.Context) (snapshot, error) {
	if c.definition.Mode == modePersistent {
		return c.collectPersistent(ctx)
	}
	return c.collectOneshot(ctx)
}

func (c *Collector) collectOneshot(ctx context.Context) (snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	data, err := runOneshot(ctx, c.definition.command, opCollect, c.configEnvelope)
	if err != nil {
		return snapshot{}, err
	}
	var snap snapshot
	if c.definition.SnapshotFormat == formatLines {
		snap, err = decodeLines(data)
	} else {
		snap, err = decodeSnapshot(data)
	}
	if err != nil {
		return snapshot{}, fmt.Errorf("invalid collect response: %w", err)
	}
	return snap, nil
}
