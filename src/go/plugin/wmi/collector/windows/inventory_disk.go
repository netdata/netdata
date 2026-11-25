// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const queryDiskExtents = "SELECT Dependent, StartingAddress, EndingAddress FROM Win32_LogicalDiskToPartition"

// WMI may return either an absolute or a relative CIM object path.
var logicalDiskReference = regexp.MustCompile(
	`(?i)^(?:(?:\\\\[^\\]+\\)?root\\cimv2:)?Win32_LogicalDisk\.DeviceID="([a-z]:)"$`,
)

// diskCapacity sums the partition extents of the fixed drives, in bytes. It
// returns an empty value unless every fixed drive letter maps to exactly one
// partition extent, so an ambiguous layout is reported as unknown.
func diskCapacity(ctx context.Context, session querySession, fixedDrives []map[string]any) (string, error) {
	if len(fixedDrives) == 0 {
		return "", nil
	}
	mapped := make(map[string]bool, len(fixedDrives))
	for _, row := range fixedDrives {
		id, _ := row["deviceid"].(string)
		id = strings.ToUpper(id)
		if len(id) != 2 || id[0] < 'A' || id[0] > 'Z' || id[1] != ':' {
			return "", nil
		}
		if _, duplicate := mapped[id]; duplicate {
			return "", nil
		}
		mapped[id] = false
	}

	rows, err := session.Query(ctx, queryDiskExtents)
	if err != nil {
		return "", err
	}
	unmapped := len(mapped)
	var total uint64
	for _, row := range rows {
		path, _ := row["dependent"].(string)
		match := logicalDiskReference.FindStringSubmatch(path)
		if match == nil {
			return "", nil
		}
		id := strings.ToUpper(match[1])
		seen, fixed := mapped[id]
		if !fixed {
			continue
		}
		start := readUnsigned(row["startingaddress"])
		end := readUnsigned(row["endingaddress"])
		if seen || !start.valid || !end.valid || start.value > end.value {
			return "", nil
		}
		// The extent includes its ending byte. Check both that addition and
		// the aggregate before proceeding.
		length := end.value - start.value
		if length >= math.MaxUint64-total {
			return "", nil
		}
		total += length + 1
		mapped[id] = true
		unmapped--
	}
	if unmapped != 0 {
		return "", nil
	}
	return strconv.FormatUint(total, 10), nil
}
