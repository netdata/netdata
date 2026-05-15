// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import "log/slog"

func jobLoggerAttrs(collector, job, source string) []any {
	return []any{
		slog.String("collector", collector),
		slog.String("job", job),
		slog.String("config_source", source),
	}
}
