// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main holds main related files
package main

import (
	"flag"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition/schema"
	"os"
)

//go:generate go run ./main.go ../schema/profile_rc_schema.json

func main() {
	var output string

	flag.StringVar(&output, "output", "../schema/profile_rc_schema.json", "Generate JSON schema generated file")
	flag.Parse()

	schemaJSON, err := schema.GenerateJSONSchema()
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(output, schemaJSON, 0664)
	if err != nil {
		panic(err)
	}

	fmt.Printf("File generated at %s\n", output)
}
