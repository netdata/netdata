// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (f *Filecheck) validateConfig() error {
	if len(f.Files.Include) == 0 && len(f.Dirs.Include) == 0 {
		return errors.New("both 'files->include' and 'dirs->include' are empty")
	}
	return nil
}

func (f *Filecheck) initFilesFilter() (matcher.Matcher, error) {
	return newFilter(f.Files.Exclude)
}

func (f *Filecheck) initDirsFilter() (matcher.Matcher, error) {
	return newFilter(f.Dirs.Exclude)
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
