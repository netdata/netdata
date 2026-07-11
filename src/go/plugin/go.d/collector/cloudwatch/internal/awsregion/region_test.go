// SPDX-License-Identifier: GPL-3.0-or-later

package awsregion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValid(t *testing.T) {
	for _, region := range []string{"us-east-1", "cn-north-1", "us-gov-west-1", "eusc-de-east-1"} {
		t.Run(region, func(t *testing.T) { assert.True(t, Valid(region)) })
	}
	for _, region := range []string{"", "global", "us-east", "us east 1"} {
		t.Run(region, func(t *testing.T) { assert.False(t, Valid(region)) })
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
