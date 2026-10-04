// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"regexp"
	"strconv"
)

var (
	tracingOriginRe = regexp.MustCompile(`^https?://[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*(:[0-9]{1,5})?$`)
	exportToRe      = regexp.MustCompile(`^(https://)?[A-Za-z0-9.-]+:[0-9]{1,5}$`)
)

// ValidateSiteExtras checks the bot and tracing settings,
// shared by file config and dynamic configuration.
func ValidateSiteExtras(s Site) []string {
	var errs []string
	if s.Bots != "" && s.Bots != BotsExclude && s.Bots != BotsInclude {
		errs = append(errs, "bots must be exclude or include")
	}
	if t := s.Tracing; t != nil {
		for _, o := range t.PropagateTo {
			if !tracingOriginRe.MatchString(o) {
				errs = append(errs, "tracing.propagate_to entry "+o+" must be http(s)://host[:port]")
			}
		}
		if t.ExportTo != "" && !exportToRe.MatchString(t.ExportTo) {
			errs = append(errs, "tracing.export_to must be host:port or https://host:port")
		}
	}
	for _, r := range s.RedactPaths {
		if r.Pattern == "" {
			continue // an empty row from the form: ignored, never matched
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			errs = append(errs, "redact_paths pattern "+strconv.Quote(r.Pattern)+" is not a valid regular expression")
		}
	}
	return errs
}
