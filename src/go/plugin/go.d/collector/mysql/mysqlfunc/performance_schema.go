// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"fmt"
	"strings"
)

const querySelectPerformanceSchema = "SELECT @@performance_schema"

func isPerformanceSchemaEnabled(ctx context.Context, deps Deps) (bool, error) {
	db, err := deps.DB()
	if err != nil {
		return false, err
	}

	var value string
	if err := db.QueryRowContext(ctx, querySelectPerformanceSchema).Scan(&value); err != nil {
		return false, fmt.Errorf("failed to query performance_schema setting: %w", err)
	}

	return strings.EqualFold(value, "ON") || value == "1", nil
}
