// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func main() {
	var schemaPath string
	var inputPath string
	var minRows int
	var requireRows bool

	flag.StringVar(&schemaPath, "schema", "", "schema file path (optional)")
	flag.StringVar(&inputPath, "input", "", "input JSON file (default: stdin)")
	flag.IntVar(&minRows, "min-rows", 0, "minimum rows required in data responses (0 disables row check)")
	flag.BoolVar(&requireRows, "require-rows", false, "require at least one row in data responses")
	flag.Parse()

	schemaBytes, err := loadSchema(schemaPath)
	if err != nil {
		exitErr("load schema: %v", err)
	}

	inputBytes, err := loadInput(inputPath)
	if err != nil {
		exitErr("load input: %v", err)
	}

	var payload any
	if err := json.Unmarshal(inputBytes, &payload); err != nil {
		exitErr("parse input JSON: %v", err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", bytes.NewReader(schemaBytes)); err != nil {
		exitErr("add schema resource: %v", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		exitErr("compile schema: %v", err)
	}

	if err := schema.Validate(payload); err != nil {
		exitErr("validation failed: %v", err)
	}

	if requireRows && minRows == 0 {
		minRows = 1
	}
	if minRows > 0 {
		rows, err := countRows(payload)
		if err != nil {
			exitErr("row check failed: %v", err)
		}
		if rows < minRows {
			exitErr("row check failed: expected at least %d rows, got %d", minRows, rows)
		}
	}
}

func countRows(payload any) (int, error) {
	obj, ok := payload.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("expected JSON object")
	}

	if errMsg, ok := obj["errorMessage"]; ok {
		if s, ok := errMsg.(string); ok && s != "" {
			return 0, fmt.Errorf("error response: %s", s)
		}
		return 0, fmt.Errorf("error response without message")
	}

	data, ok := obj["data"]
	if !ok {
		return 0, fmt.Errorf("missing data field")
	}

	rows, ok := data.([]any)
	if !ok {
		return 0, fmt.Errorf("data is not an array")
	}

	return len(rows), nil
}

func loadSchema(path string) ([]byte, error) {
	if path == "" {
		path = filepath.Clean(filepath.Join("..", "plugins.d", "FUNCTION_UI_SCHEMA.json"))
	}
	return os.ReadFile(path)
}

func loadInput(path string) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func exitErr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
