// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cmd implements a cobra command for validating and normalizing profiles.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "normalize_cmd FILE [FILE...]",
	Short: "Validate and normalize profiles.",
	Long: `normalize_cmd is a tool for validating and normalizing profiles.

	Each profile file passed in will be parsed, and any errors in them will be
	reported. If an output directory is specified with -o, then the profiles will
	also be normalized, migrating legacy and deprecated structures to their
	modern counterparts, and written to the output directory.`,

	Run: func(cmd *cobra.Command, args []string) {
		outdir, err := cmd.Flags().GetString("outdir")
		if err != nil {
			fmt.Printf("parse failure %v, unable to generate output\n", err)
		}
		useJSON, err := cmd.Flags().GetBool("json")
		if err != nil {
			fmt.Printf("parse failure %v, unable to generate output\n", err)
		}
		for _, filePath := range args {
			filename := filepath.Base(filePath)
			name := filename[:len(filename)-len(filepath.Ext(filename))] // remove extension
			def, errors := GetProfile(filePath)
			if len(errors) > 0 {
				fmt.Printf("*** %d error(s) in profile %q ***\n", len(errors), filePath)
				for _, e := range errors {
					fmt.Println("  ", e)
				}
				fmt.Println()
				continue
			}
			if err := WriteProfile(def, name, outdir, useJSON); err != nil {
				fmt.Println(err)
			}
		}
	},
}

// GetProfile parses a profile from a file path and validates it.
func GetProfile(filePath string) (*profiledefinition.ProfileDefinition, []string) {
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return nil, []string{fmt.Sprintf("unable to read file: %v", err)}
	}
	def := profiledefinition.NewProfileDefinition()
	err = yaml.Unmarshal(buf, def)
	if err != nil {
		return nil, []string{fmt.Sprintf("unable to parse profile: %v", err)}
	}
	errors := profiledefinition.ValidateEnrichProfile(def)
	if len(errors) > 0 {
		return nil, errors
	}
	return def, nil
}

// WriteProfile writes a profile to disk.
func WriteProfile(def *profiledefinition.ProfileDefinition, name string, outdir string, useJSON bool) error {
	if outdir == "" {
		return nil
	}
	var filename string
	var data []byte
	if useJSON {
		var err error
		filename = name + ".json"
		data, err = json.Marshal(def)
		if err != nil {
			return fmt.Errorf("unable to marshal profile %s: %w", name, err)
		}
	} else {
		var err error
		data, err = yaml.Marshal(def)
		filename = name + ".yaml"
		if err != nil {
			return fmt.Errorf("unable to marshal profile %s: %w", name, err)
		}
	}
	outfile := filepath.Join(outdir, filename)
	f, err := os.Create(outfile)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", outfile, err)
	}
	defer func() {
		_ = f.Close()
	}()
	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("unable to write to file %s: %w", outfile, err)
	}
	return nil
}

// Execute runs the command.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("outdir", "o", "", "Output path for normalized files. If blank, inputs will be validated but not output.")
	rootCmd.Flags().BoolP("json", "j", false, "Output as JSON")
}
