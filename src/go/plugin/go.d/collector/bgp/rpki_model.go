// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

type rpkiCacheStats struct {
	ID          string
	Backend     string
	Name        string
	Desc        string
	StateText   string
	Up          bool
	HasUptime   bool
	UptimeSecs  int64
	HasRecords  bool
	RecordIPv4  int64
	RecordIPv6  int64
	HasPrefixes bool
	PrefixIPv4  int64
	PrefixIPv6  int64
}

type rpkiInventoryStats struct {
	ID         string
	Backend    string
	Scope      string
	PrefixIPv4 int64
	PrefixIPv6 int64
}
