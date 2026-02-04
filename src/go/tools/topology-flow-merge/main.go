// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

type functionResponse struct {
	Status int             `json:"status"`
	Type   string          `json:"type"`
	Data   json.RawMessage `json:"data"`
}

func main() {
	var outputType string
	flag.StringVar(&outputType, "type", "auto", "auto|topology|flows|both")
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "usage: topology-flow-merge [--type=auto|topology|flows|both] <file1.json> <file2.json> ...")
		os.Exit(1)
	}

	var topologies []topologyData
	var flows []flowsData

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read %s: %v\n", file, err)
			os.Exit(1)
		}

		var resp functionResponse
		if err := json.Unmarshal(content, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse %s: %v\n", file, err)
			os.Exit(1)
		}
		if resp.Status >= 400 {
			fmt.Fprintf(os.Stderr, "skipping %s: status %d\n", file, resp.Status)
			continue
		}

		switch resp.Type {
		case "topology":
			var data topologyData
			if err := json.Unmarshal(resp.Data, &data); err != nil {
				fmt.Fprintf(os.Stderr, "failed to parse topology data in %s: %v\n", file, err)
				os.Exit(1)
			}
			topologies = append(topologies, data)
		case "flows":
			var data flowsData
			if err := json.Unmarshal(resp.Data, &data); err != nil {
				fmt.Fprintf(os.Stderr, "failed to parse flows data in %s: %v\n", file, err)
				os.Exit(1)
			}
			flows = append(flows, data)
		default:
			fmt.Fprintf(os.Stderr, "skipping %s: unknown type %q\n", file, resp.Type)
		}
	}

	result := buildOutput(outputType, topologies, flows)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
		os.Exit(1)
	}
}

func buildOutput(outputType string, topologies []topologyData, flows []flowsData) any {
	switch outputType {
	case "topology":
		return mergeTopology(topologies)
	case "flows":
		return mergeFlows(flows)
	case "both":
		return map[string]any{
			"topology": mergeTopology(topologies),
			"flows":    mergeFlows(flows),
		}
	default:
		switch {
		case len(topologies) > 0 && len(flows) > 0:
			return map[string]any{
				"topology": mergeTopology(topologies),
				"flows":    mergeFlows(flows),
			}
		case len(topologies) > 0:
			return mergeTopology(topologies)
		case len(flows) > 0:
			return mergeFlows(flows)
		default:
			return map[string]any{}
		}
	}
}
