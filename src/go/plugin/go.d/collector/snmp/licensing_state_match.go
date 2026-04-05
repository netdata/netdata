// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"strings"
	"unicode"
)

var (
	licenseStateIgnoredHints = newLicenseStateHintSet(
		"ignored",
		"not_subscribed",
		"not subscribed",
		"not-applicable",
		"not_applicable",
		"not applicable",
		"none",
		"n/a",
	)
	licenseStateBrokenHints = newLicenseStateHintSet(
		"expired",
		"expired_in_use",
		"expired_not_in_use",
		"authorization_expired",
		"grace_period_expired",
		"evaluation_expired",
		"usage_count_consumed",
		"invalid",
		"invalid_tag",
		"unauthorized",
		"not_authorized",
		"not authorized",
		"out-of-compliance",
		"out_of_compliance",
		"not-associated",
		"not_associated",
		"not associated",
		"disabled",
		"deactivated",
		"failed",
		"error",
		"violation",
		"broken",
	)
	licenseStateDegradedHints = newLicenseStateHintSet(
		"about-to-expire",
		"about_to_expire",
		"about to expire",
		"warning",
		"degrade",
		"grace",
		"evaluation",
		"evaluation_subscription",
		"evaluation subscription",
		"evaluation_period",
		"evaluation period",
		"eval",
		"trial",
		"overage",
		"partial",
		"unknown",
		"initialized",
		"waiting",
	)
	licenseStateHealthyHints = newLicenseStateHintSet(
		"valid",
		"active",
		"authorized",
		"reserved_authorized",
		"reserved authorized",
		"compliant",
		"ok",
		"subscribed",
		"registered",
		"in_use",
		"in-use",
		"in use",
		"up-to-date",
		"up_to_date",
		"up to date",
	)
)

func newLicenseStateHintSet(hints ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(hints))
	for _, hint := range hints {
		if normalized := normalizeLicenseStateText(hint); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
}

func normalizeLicenseStateText(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, raw)

	return strings.Join(strings.Fields(normalized), " ")
}

func licenseStateMatchesAny(raw string, hints map[string]struct{}) bool {
	if len(hints) == 0 {
		return false
	}

	normalized := normalizeLicenseStateText(raw)
	if normalized == "" {
		return false
	}

	if _, ok := hints[normalized]; ok {
		return true
	}

	// Match normalized phrases on token boundaries so "authorization expired due to policy"
	// still matches the broken hint "authorization expired", without reintroducing false
	// positives such as "inactive" matching "active".
	padded := " " + normalized + " "
	for hint := range hints {
		if hint == "" || hint == normalized {
			continue
		}
		if strings.Contains(padded, " "+hint+" ") {
			return true
		}
	}

	return false
}
