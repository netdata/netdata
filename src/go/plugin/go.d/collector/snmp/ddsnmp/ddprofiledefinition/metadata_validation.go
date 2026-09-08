// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
)

var validDeviceMetadataFields = map[string]struct{}{
	"name":                        {},
	"description":                 {},
	"sys_object_id":               {},
	"location":                    {},
	"serial_number":               {},
	"vendor":                      {},
	"version":                     {},
	"software_version":            {},
	"firmware_version":            {},
	"hardware_version":            {},
	"product_name":                {},
	"model":                       {},
	"os_name":                     {},
	"os_version":                  {},
	"os_hostname":                 {},
	"category":                    {},
	"type":                        {},
	"lldp_loc_chassis_id":         {},
	"lldp_loc_chassis_id_subtype": {},
	"lldp_loc_sys_name":           {},
	"lldp_loc_sys_desc":           {},
	"lldp_loc_sys_cap_supported":  {},
	"lldp_loc_sys_cap_enabled":    {},
	"bridge_base_address":         {},
	"ospf_router_id":              {},
}

func validateEnrichMetadata(metadata MetadataConfig) error {
	var errs []error

	for resName := range metadata {
		if resName != MetadataDeviceResource {
			errs = append(errs, fmt.Errorf("invalid resource: %s", resName))
		} else {
			res := metadata[resName]
			for fieldName := range res.Fields {
				if !isValidMetadataField(fieldName) {
					errs = append(errs, fmt.Errorf("invalid resource (%s) field: %s", resName, fieldName))
					continue
				}
				field := res.Fields[fieldName]
				errs = append(errs, validateConsumers(fmt.Sprintf("metadata.%s.fields.%s.consumers", resName, fieldName), field.Consumers))
				for i := range field.Symbols {
					errs = append(errs, validateEnrichSymbol(&field.Symbols[i], metadataSymbol))
				}
				if field.Symbol.OID != "" {
					errs = append(errs, validateEnrichSymbol(&field.Symbol, metadataSymbol))
				}
				res.Fields[fieldName] = field
			}
			metadata[resName] = res
		}
	}

	return errors.Join(errs...)
}

func validateEnrichSysobjectIDMetadata(entries []SysobjectIDMetadataEntryConfig) error {
	var errs []error

	// Track seen sysobjectids to detect duplicates
	seenOIDs := make(map[string]int) // OID -> first occurrence index

	for i, entry := range entries {
		// Validate sysobjectid is not empty
		if entry.SysobjectID == "" {
			errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d]: missing sysobjectid", i))
			continue
		}

		// Check for duplicate sysobjectids
		if firstIdx, exists := seenOIDs[entry.SysobjectID]; exists {
			errs = append(
				errs,
				fmt.Errorf("sysobjectid_metadata[%d]: duplicate sysobjectid %s (first occurrence at index %d)",
					i, entry.SysobjectID, firstIdx),
			)
		} else {
			seenOIDs[entry.SysobjectID] = i
		}

		// Validate metadata fields
		for fieldName, field := range entry.Metadata {
			if !isValidMetadataField(fieldName) {
				errs = append(
					errs,
					fmt.Errorf(
						"sysobjectid_metadata[%d]: invalid resource (%s) field: %s",
						i,
						MetadataDeviceResource,
						fieldName,
					),
				)
				continue
			}

			// Validate the field must have either value or symbol(s)
			if field.Value == "" && field.Symbol.OID == "" && len(field.Symbols) == 0 {
				errs = append(
					errs,
					fmt.Errorf("sysobjectid_metadata[%d].%s: must have either value or symbol(s)", i, fieldName),
				)
			}
			errs = append(
				errs,
				validateConsumers(fmt.Sprintf("sysobjectid_metadata[%d].%s.consumers", i, fieldName), field.Consumers),
			)

			// Can't have both value and symbols
			if field.Value != "" && (field.Symbol.OID != "" || len(field.Symbols) > 0) {
				errs = append(
					errs,
					fmt.Errorf("sysobjectid_metadata[%d].%s: cannot have both value and symbol(s)", i, fieldName),
				)
			}

			// Validate symbols if present
			for j := range field.Symbols {
				if err := validateEnrichSymbol(&field.Symbols[j], metadataSymbol); err != nil {
					errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d].%s.symbols[%d]: %s", i, fieldName, j, err))
				}
			}

			// Validate single symbol if present
			if field.Symbol.OID != "" {
				if err := validateEnrichSymbol(&field.Symbol, metadataSymbol); err != nil {
					errs = append(errs, fmt.Errorf("sysobjectid_metadata[%d].%s.symbol: %s", i, fieldName, err))
				}
			}

			entry.Metadata[fieldName] = field
		}
	}

	return errors.Join(errs...)
}

func isValidMetadataField(fieldName string) bool {
	_, ok := validDeviceMetadataFields[fieldName]
	return ok
}
