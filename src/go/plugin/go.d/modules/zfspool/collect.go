// SPDX-License-Identifier: GPL-3.0-or-later

package zfspool

var zpoolHealthStates = []string{
	"online",
	"degraded",
	"faulted",
	"offline",
	"removed",
	"unavail",
	"suspended",
}

func (z *ZFSPool) collect() (map[string]int64, error) {

	mx := make(map[string]int64)

	if err := z.collectZpoolList(mx); err != nil {
		return nil, err
	}
	if err := z.collectZpoolListVdev(mx); err != nil {
		return mx, err
	}

	return mx, nil
}
