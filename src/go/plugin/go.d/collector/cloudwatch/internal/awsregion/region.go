// SPDX-License-Identifier: GPL-3.0-or-later

package awsregion

import (
	"regexp"
	"strings"
)

var codePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)+-[0-9]+$`)

// Normalize returns the canonical form used by AWS region identifiers.
func Normalize(region string) string {
	return strings.ToLower(strings.TrimSpace(region))
}

// Valid reports whether region has the shape of an AWS region identifier.
func Valid(region string) bool {
	return codePattern.MatchString(Normalize(region))
}

// Partition returns the AWS partition selected by region.
func Partition(region string) string {
	region = Normalize(region)
	switch {
	case strings.HasPrefix(region, "us-gov-"):
		return "aws-us-gov"
	case strings.HasPrefix(region, "cn-"):
		return "aws-cn"
	case strings.HasPrefix(region, "us-isob-"):
		return "aws-iso-b"
	case strings.HasPrefix(region, "us-isof-"):
		return "aws-iso-f"
	case strings.HasPrefix(region, "us-iso-"):
		return "aws-iso"
	case strings.HasPrefix(region, "eu-isoe-"):
		return "aws-iso-e"
	case strings.HasPrefix(region, "eusc-"):
		return "aws-eusc"
	default:
		return "aws"
	}
}
