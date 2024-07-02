// SPDX-License-Identifier: GPL-3.0-or-later

package confgroup

type Registry map[string]Default

type Default struct {
	MinUpdateEvery     int `yaml:"-"`
	UpdateEvery        int `yaml:"update_every"`
	AutoDetectionRetry int `yaml:"autodetection_retry"`
	Priority           int `yaml:"priority"`
}

func (r Registry) Register(name string, def Default) {
	if name != "" {
		r[name] = def
	}
}

func (r Registry) Lookup(name string) (Default, bool) {
	def, ok := r[name]
	return def, ok
}
