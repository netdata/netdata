// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaTypeValidationRequiresExactResourceAndCollectionNamespaces(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		validate func() error
		valid    bool
	}{
		"versioned resource": {
			validate: func() error {
				return validateResourceSchemaType("system", "#ComputerSystem.v1_25_0.ComputerSystem")
			},
			valid: true,
		},
		"versionless resource": {
			validate: func() error {
				return validateResourceSchemaType("system", "#ComputerSystem.ComputerSystem")
			},
		},
		"wrong resource": {
			validate: func() error {
				return validateResourceSchemaType("system", "#Chassis.v1_25_0.Chassis")
			},
		},
		"missing errata version": {
			validate: func() error {
				return validateResourceSchemaType("system", "#ComputerSystem.v1_25.ComputerSystem")
			},
		},
		"nonnumeric version": {
			validate: func() error {
				return validateResourceSchemaType("system", "#ComputerSystem.v1_x_0.ComputerSystem")
			},
		},
		"extra version component": {
			validate: func() error {
				return validateResourceSchemaType("system", "#ComputerSystem.v1_25_0_1.ComputerSystem")
			},
		},
		"exact collection": {
			validate: func() error {
				return validateCollectionSchemaType(
					"#ComputerSystemCollection.ComputerSystemCollection",
					"system",
				)
			},
			valid: true,
		},
		"versioned exact collection": {
			validate: func() error {
				return validateCollectionSchemaType(
					"#ComputerSystemCollection.v1_0_0.ComputerSystemCollection",
					"system",
				)
			},
		},
		"wrong collection": {
			validate: func() error {
				return validateCollectionSchemaType(
					"#ChassisCollection.ChassisCollection",
					"system",
				)
			},
		},
		"generic collection": {
			validate: func() error {
				return validateCollectionSchemaType(
					"#ResourceCollection.ResourceCollection",
					"system",
				)
			},
		},
		"malformed collection version": {
			validate: func() error {
				return validateCollectionSchemaType(
					"#ComputerSystemCollection.v1_0.ComputerSystemCollection",
					"system",
				)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := test.validate()
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestSchemaAndRedfishVersionParsingRejectsOversizedTokensBeforeSplitting(t *testing.T) {
	odataType := "#" + strings.Repeat("x.", 1<<20) + "ComputerSystem"
	name, namespace, ok := parseODataType(odataType)
	require.False(t, ok)
	require.Empty(t, name)
	require.Empty(t, namespace)
	require.Error(t, validateResourceSchemaType("system", odataType))

	version := strings.Repeat("1.", 1<<20) + "0"
	require.False(t, validRedfishVersion(version))
	require.False(t, validVersionedSchemaNamespace("ComputerSystem", "ComputerSystem.v"+version))
}

func TestRequiredResourceProperties(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		validate func() error
		valid    bool
	}{
		"resource complete": {
			validate: func() error {
				return validateRequiredResourceProperties("fan", map[string]any{"Id": "1", "Name": "Fan 1"})
			},
			valid: true,
		},
		"resource missing Id": {
			validate: func() error {
				return validateRequiredResourceProperties("fan", map[string]any{"Name": "Fan 1"})
			},
		},
		"resource missing Name": {
			validate: func() error {
				return validateRequiredResourceProperties("fan", map[string]any{"Id": "1"})
			},
		},
		"inline LogEntry complete": {
			validate: func() error {
				return validateRequiredLogEntryProperties(map[string]any{
					"Name": "Entry 1", "EntryType": "Event",
				}, false)
			},
			valid: true,
		},
		"linked LogEntry requires Id": {
			validate: func() error {
				return validateRequiredLogEntryProperties(map[string]any{
					"Name": "Entry 1", "EntryType": "Event",
				}, true)
			},
		},
		"LogEntry requires Name": {
			validate: func() error {
				return validateRequiredLogEntryProperties(map[string]any{
					"Id": "1", "EntryType": "Event",
				}, true)
			},
		},
		"LogEntry requires EntryType": {
			validate: func() error {
				return validateRequiredLogEntryProperties(map[string]any{
					"Id": "1", "Name": "Entry 1",
				}, true)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := test.validate()
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
