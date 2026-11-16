// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"errors"
	"fmt"
	"strconv"
)

// defaultRangeSeconds is the span of a history range whose after is omitted.
const defaultRangeSeconds = 15 * 60

// historyRangeHelp describes the after and before arguments of historyRange.
var historyRangeHelp = "Pass after and before (Unix seconds, or negative seconds from now) to set the time range; " +
	"before defaults to now and after to " + strconv.Itoa(defaultRangeSeconds/60) + " minutes before it."

func historyRange(args map[string]string, now int64) (int64, int64, error) {
	var bounds [2]int64
	for i, key := range []string{"after", "before"} {
		if args[key] != "" {
			value, err := strconv.ParseInt(args[key], 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("%s must be Unix seconds or a relative negative offset", key)
			}
			if value < 0 {
				value += now
			}
			bounds[i] = value
		}
	}
	if args["before"] == "" {
		bounds[1] = now
	}
	if args["after"] == "" {
		bounds[0] = max(0, bounds[1]-defaultRangeSeconds)
	}
	if bounds[0] < 0 || bounds[1] < 0 {
		return 0, 0, errors.New("history bounds must not precede epoch")
	}
	if bounds[0] > bounds[1] {
		return 0, 0, errors.New("after must not exceed before")
	}
	return bounds[0], bounds[1], nil
}
