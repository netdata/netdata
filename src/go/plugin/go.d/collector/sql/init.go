// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"errors"
	"fmt"
)

func (c *Collector) verifyConfig() error {
	var errs []error

	if c.Driver == "" {
		errs = append(errs, errors.New("driver required"))
	}
	if !supportedDrivers[c.Driver] {
		errs = append(errs, errors.New("unsupported driver"))
	}

	if c.DSN == "" {
		errs = append(errs, errors.New("missing dsn"))
	}

	if len(c.Queries) == 0 {
		errs = append(errs, errors.New("missing queries"))
	}

	for i, q := range c.Queries {
		i++
		if q.Name == "" {
			errs = append(errs, fmt.Errorf("queries[%d] missing name", i))
		}
		if q.Query == "" {
			errs = append(errs, fmt.Errorf("queries[%d] missing query", i))
		}
		if len(q.Values) == 0 {
			errs = append(errs, fmt.Errorf("queries[%d] missing values", i))
		}
	}

	return errors.Join(errs...)
}
