// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

// Options contains only endpoint acquisition settings. The caller validates public
// configuration and owns HTTP setup, scheduling and measurement conversion.
type Options struct {
	URL                   string
	AuthMethod            string
	Username              string
	Password              string
	MaxConcurrentRequests int
	Collect               string
}

var collectionFamilies = []string{
	"base",
	"compute",
	"memory",
	"thermal",
	"power",
	"storage",
	"network",
	"pcie",
	"sensors",
	"firmware",
}

func ValidateCollectionPattern(expr string) error {
	if _, err := matcher.NewSimplePatternsMatcher(expr); err != nil {
		return err
	}
	for term := range strings.FieldsSeq(expr) {
		term = strings.TrimPrefix(term, "!")
		if term == "" {
			return errors.New("contains an empty pattern")
		}
		m, err := matcher.NewSimplePatternsMatcher(term)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(collectionFamilies, m.MatchString) {
			return fmt.Errorf("pattern %q matches no supported collection family", term)
		}
	}
	return nil
}
