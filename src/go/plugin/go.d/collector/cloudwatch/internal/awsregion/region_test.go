// SPDX-License-Identifier: GPL-3.0-or-later

package awsregion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValid(t *testing.T) {
	tests := map[string]bool{
		"us-east-1": true, "cn-north-1": true, "us-gov-west-1": true, "eusc-de-east-1": true,
		"": false, "global": false, "us-east": false, "us east 1": false,
	}
	for region, want := range tests {
		t.Run(region, func(t *testing.T) { assert.Equal(t, want, Valid(region)) })
	}
}

func TestPartition(t *testing.T) {
	tests := map[string]string{
		"us-east-1": "aws", "cn-north-1": "aws-cn", "us-gov-west-1": "aws-us-gov",
		"us-iso-east-1": "aws-iso", "us-isob-east-1": "aws-iso-b", "us-isof-south-1": "aws-iso-f",
		"eu-isoe-west-1": "aws-iso-e", "eusc-de-east-1": "aws-eusc",
	}
	for region, want := range tests {
		t.Run(region, func(t *testing.T) { assert.Equal(t, want, Partition(region)) })
	}
}
