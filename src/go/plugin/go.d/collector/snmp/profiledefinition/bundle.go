// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

// ProfileBundleProfileItem represent a profile entry with metadata.
type ProfileBundleProfileItem struct {
	Profile ProfileDefinition `json:"profile"`
}

// ProfileBundle represent a list of profiles meant to be downloaded by user.
type ProfileBundle struct {
	CreatedTimestamp int64                      `json:"created_timestamp"` // Millisecond
	Profiles         []ProfileBundleProfileItem `json:"profiles"`
}
