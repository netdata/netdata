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
				errs = append(errs, validateEnrichMetadataField(fmt.Sprintf("metadata.%s.fields.%s", resName, fieldName), &field))
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

			path := fmt.Sprintf("sysobjectid_metadata[%d].%s", i, fieldName)
			errs = append(errs, validateEnrichMetadataField(path, &field))
			if field.Value != "" && (isSymbolConfigured(field.Symbol) || len(field.Symbols) > 0) {
				errs = append(errs, fmt.Errorf("%s: cannot have both value and symbol(s)", path))
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

func validateEnrichMetadataField(path string, field *MetadataField) error {
	var errs []error
	if field.Value == "" && !isSymbolConfigured(field.Symbol) && len(field.Symbols) == 0 {
		errs = append(errs, fmt.Errorf("%s: must have either value or symbol(s)", path))
	}
	errs = append(errs, validateConsumers(path+".consumers", field.Consumers))
	if isSymbolConfigured(field.Symbol) {
		errs = append(errs, withValidationPath(path+".symbol", validateEnrichSymbol(&field.Symbol, metadataSymbol)))
	}
	for i := range field.Symbols {
		errs = append(
			errs,
			withValidationPath(
				fmt.Sprintf("%s.symbols[%d]", path, i),
				validateEnrichSymbol(&field.Symbols[i], metadataSymbol),
			),
		)
	}
	return errors.Join(errs...)
}
