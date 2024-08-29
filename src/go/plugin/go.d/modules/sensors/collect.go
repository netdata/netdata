// SPDX-License-Identifier: GPL-3.0-or-later

package sensors

const precision = 1000

func (s *Sensors) collect() (map[string]int64, error) {
	if s.exec != nil {
		return s.collectExec()
	}
	return s.collectSysfs()
}
