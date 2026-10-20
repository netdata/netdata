// SPDX-License-Identifier: GPL-3.0-or-later

// Build this directory, then deploy only the resulting executable.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

//go:embed description.yaml
var description []byte

func run() error {
	args := os.Args[1:]
	persistent := len(args) > 0 && args[0] == "--persistent"
	if persistent {
		args = args[1:]
	}
	if len(args) != 1 {
		return fmt.Errorf("expected describe, collect, function or serve")
	}
	if args[0] == "describe" {
		data := description
		if persistent {
			data = bytes.Replace(data, []byte("mode: oneshot"), []byte("mode: persistent"), 1)
		}
		_, err := os.Stdout.Write(data)
		return err
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	var config struct {
		Config struct {
			Count int `json:"count"`
		} `json:"config"`
	}
	if err := decoder.Decode(&config); err != nil {
		return err
	}
	snapshot := map[string]any{
		"version": "v1",
		"metrics": []any{map[string]any{"name": "depth", "value": config.Config.Count}},
	}
	if args[0] == "collect" {
		return encoder.Encode(snapshot)
	}
	if args[0] != "serve" && args[0] != "function" {
		return fmt.Errorf("unsupported operation")
	}
	if args[0] == "serve" {
		if err := encoder.Encode(map[string]any{"version": "v1", "ready": true}); err != nil {
			return err
		}
	}
	for {
		var request struct {
			ID       string `json:"id"`
			Method   string `json:"method"`
			Function string `json:"function"`
			Info     bool   `json:"info"`
		}
		if err := decoder.Decode(&request); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		result := snapshot
		if request.Method == "function" {
			result = map[string]any{"version": "v1", "status": 200}
			if request.Function != "items" {
				result["status"], result["message"] = 404, "Unknown Function"
			} else if !request.Info {
				result["columns"] = map[string]any{"jobs": map[string]any{"index": 0, "name": "Jobs", "type": "integer"}}
				result["data"] = [][]int{{config.Config.Count}}
			}
		}
		if err := encoder.Encode(map[string]any{"id": request.ID, "result": result}); err != nil {
			return err
		}
		if args[0] == "function" {
			return nil
		}
	}
}

func main() {
	if run() != nil {
		// Inputs can contain secrets; do not print decoding errors or payloads.
		fmt.Fprintln(os.Stderr, "native example operation failed")
		os.Exit(1)
	}
}
