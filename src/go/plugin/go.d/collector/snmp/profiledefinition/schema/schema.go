// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package schema

import _ "embed"

var (
	//go:embed profile_rc_schema.json
	deviceProfileRcConfigJsonschema []byte
)

// GetDeviceProfileRcConfigJsonschema returns deviceProfileRcConfigJsonschema
func GetDeviceProfileRcConfigJsonschema() []byte {
	return deviceProfileRcConfigJsonschema
}
