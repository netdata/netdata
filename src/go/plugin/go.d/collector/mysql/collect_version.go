// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
)

const queryShowVersion = `
SHOW GLOBAL VARIABLES 
WHERE 
  Variable_name LIKE 'version'
  OR Variable_name LIKE 'version_comment';`

var reVersionCore = regexp.MustCompile(`^\d+\.\d+\.\d+`)

func (c *Collector) collectVersion() error {
	// https://mariadb.com/kb/en/version/
	q := queryShowVersion
	c.Debugf("executing query: '%s'", queryShowVersion)

	var name, version, versionComment string
	_, err := c.collectQuery(q, func(column, value string, _ bool) {
		switch column {
		case "Variable_name":
			name = value
		case "Value":
			switch name {
			case "version":
				version = value
			case "version_comment":
				versionComment = value
			}
		}
	})
	if err != nil {
		return err
	}

	c.Infof("application version: '%s', version_comment: '%s'", version, versionComment)

	// version string is not always valid semver (ex.: 8.0.22-0ubuntu0.20.04.2)
	s := reVersionCore.FindString(version)
	if s == "" {
		return fmt.Errorf("couldn't parse version string '%s'", version)
	}

	ver, err := semver.New(s)
	if err != nil {
		return fmt.Errorf("couldn't parse version string '%s': %v", s, err)
	}

	c.version = ver
	c.isMariaDB = strings.Contains(version, "MariaDB") || strings.Contains(versionComment, "mariadb")
	c.isPercona = strings.Contains(versionComment, "Percona")

	return nil
}
