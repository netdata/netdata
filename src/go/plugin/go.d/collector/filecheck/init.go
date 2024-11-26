// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (c *Collector) validateConfig() error {
	if len(c.Files.Include) == 0 && len(c.Dirs.Include) == 0 {
		return errors.New("both 'files->include' and 'dirs->include' are empty")
	}
	return nil
}

func (c *Collector) initFilesFilter() (matcher.Matcher, error) {
	return newFilter(c.Files.Exclude)
}

func (c *Collector) initDirsFilter() (matcher.Matcher, error) {
	return newFilter(c.Dirs.Exclude)
}

func newFilter(patterns []string) (matcher.Matcher, error) {
	filter := matcher.FALSE()

	for _, s := range patterns {
		m, err := matcher.NewGlobMatcher(s)
		if err != nil {
			return nil, err
		}
		filter = matcher.Or(filter, m)
	}

	return filter, nil
}
