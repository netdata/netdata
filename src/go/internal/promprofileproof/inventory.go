// SPDX-License-Identifier: GPL-3.0-or-later

package promprofileproof

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

const (
	inventoryAuthoredSelectorColumn = 0
	inventorySourceFamilyColumn     = 2
	inventorySourceLabelsColumn     = 4
	inventoryDispositionColumn      = 14
)

var sourceInventoryHeader = []string{
	"authored_selector",
	"emitted_metric",
	"source_family",
	"source_type",
	"source_labels",
	"operator_owner",
	"entity_identity",
	"signal_role",
	"observation_population",
	"cross_family_relationship",
	"unit_algebra",
	"label_roles_and_optionality",
	"availability_gate",
	"evidence_and_uncertainty",
	"disposition",
	"destination",
	"source_path",
}

type SourceInventory struct {
	Rows              int
	SourceFamilies    map[string]struct{}
	AuthoredSelectors map[string]struct{}
	Dispositions      InventoryDisposition
}

func LoadSourceInventory(path string) (SourceInventory, error) {
	file, err := os.Open(path)
	if err != nil {
		return SourceInventory{}, fmt.Errorf("open source inventory: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = len(sourceInventoryHeader)
	header, err := reader.Read()
	if err != nil {
		return SourceInventory{}, fmt.Errorf("read source inventory header: %w", err)
	}
	if !slices.Equal(header, sourceInventoryHeader) {
		return SourceInventory{}, fmt.Errorf("source inventory header: got %v, want %v", header, sourceInventoryHeader)
	}

	inventory := SourceInventory{
		SourceFamilies:    make(map[string]struct{}),
		AuthoredSelectors: make(map[string]struct{}),
	}
	seenRows := make(map[string]int)
	for row := 2; ; row++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return SourceInventory{}, fmt.Errorf("read source inventory row %d: %w", row, err)
		}
		for index, value := range record {
			if index == inventorySourceLabelsColumn {
				continue
			}
			if value == "" {
				return SourceInventory{}, fmt.Errorf("source inventory row %d field %s must not be empty", row, sourceInventoryHeader[index])
			}
		}
		key := strings.Join(record, "\x00")
		if previous := seenRows[key]; previous != 0 {
			return SourceInventory{}, fmt.Errorf("source inventory row %d duplicates row %d", row, previous)
		}
		seenRows[key] = row

		inventory.Rows++
		inventory.SourceFamilies[record[inventorySourceFamilyColumn]] = struct{}{}
		switch record[inventoryDispositionColumn] {
		case "chart":
			inventory.Dispositions.Chart++
			inventory.AuthoredSelectors[record[inventoryAuthoredSelectorColumn]] = struct{}{}
		case "job-excluded":
			inventory.Dispositions.JobExcluded++
		case "writer-ineligible":
			inventory.Dispositions.WriterIneligible++
		default:
			return SourceInventory{}, fmt.Errorf("source inventory row %d disposition %q is unsupported",
				row, record[inventoryDispositionColumn])
		}
	}
	if inventory.Rows == 0 {
		return SourceInventory{}, errors.New("source inventory has no data rows")
	}
	return inventory, nil
}

func (i SourceInventory) VerifyExpected(expected SourceInventoryExpected) error {
	actual := SourceInventoryExpected{
		Rows:              i.Rows,
		SourceFamilies:    len(i.SourceFamilies),
		AuthoredSelectors: len(i.AuthoredSelectors),
		Dispositions:      i.Dispositions,
	}
	for _, item := range []struct {
		field string
		got   int
		want  int
	}{
		{"rows", actual.Rows, expected.Rows},
		{"source_families", actual.SourceFamilies, expected.SourceFamilies},
		{"authored_selectors", actual.AuthoredSelectors, expected.AuthoredSelectors},
		{"dispositions.chart", actual.Dispositions.Chart, expected.Dispositions.Chart},
		{"dispositions.job_excluded", actual.Dispositions.JobExcluded, expected.Dispositions.JobExcluded},
		{"dispositions.writer_ineligible", actual.Dispositions.WriterIneligible, expected.Dispositions.WriterIneligible},
	} {
		if item.got != item.want {
			return fmt.Errorf("source inventory %s: got %d, want %d", item.field, item.got, item.want)
		}
	}
	return nil
}
