// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"sort"
	"strconv"
	"strings"
)

// vmBuildGroupKey returns a stable group key.
func vmBuildGroupKey(tags map[string]string, agg *vmetricsAggregator) (string, bool) {
	if !agg.grouped || len(tags) == 0 {
		return "", false
	}

	agg.keyBuf.Reset()

	if agg.perRow {
		if len(agg.groupBy) > 0 {
			missingHint := false
			for _, label := range agg.groupBy {
				value := tags[label]
				if value == "" {
					missingHint = true
					break
				}
				vmWriteGroupKeyPart(&agg.keyBuf, value)
			}
			if !missingHint {
				return agg.keyBuf.String(), true
			}
			agg.keyBuf.Reset()
		}

		// per-row without group_by: stable length-prefixed key from all non-underscore tags
		keys := make([]string, 0, len(tags))
		for key := range tags {
			if !strings.HasPrefix(key, "_") {
				keys = append(keys, key)
			}
		}
		if len(keys) == 0 {
			return "", false
		}

		sort.Strings(keys)
		for _, key := range keys {
			vmWriteGroupKeyPart(&agg.keyBuf, key)
			vmWriteGroupKeyPart(&agg.keyBuf, tags[key])
		}
		return agg.keyBuf.String(), true
	}

	// non per-row: respect group_by exactly; underscore labels are NOT special
	switch len(agg.groupBy) {
	case 0:
		return "", false
	case 1:
		label := agg.groupBy[0]
		value := tags[label]
		return value, value != ""
	default:
		for _, label := range agg.groupBy {
			value := tags[label]
			if value == "" {
				return "", false
			}
			vmWriteGroupKeyPart(&agg.keyBuf, value)
		}
		return agg.keyBuf.String(), true
	}
}

func vmWriteGroupKeyPart(buf *strings.Builder, value string) {
	buf.WriteString(strconv.Itoa(len(value)))
	buf.WriteByte(':')
	buf.WriteString(value)
}
