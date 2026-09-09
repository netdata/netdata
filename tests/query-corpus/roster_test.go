// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"reflect"
	"strings"
	"testing"
)

func TestGroupingRosterParserHandlesMultilineDeclarations(t *testing.T) {
	enumSource := []byte(`
typedef enum rrdr_time_grouping
{
    RRDR_GROUPING_UNDEFINED = 0,
    RRDR_GROUPING_AVERAGE
        = 1,
    /* a comma on its own line used to evade the line parser */
    RRDR_GROUPING_SUM
    ,
    RRDR_GROUPING_SENTINEL
} RRDR_TIME_GROUPING;
`)
	registrySource := []byte(`
static struct grouping api_v1_data_groups[] = {
    {
        .value =
            RRDR_GROUPING_AVERAGE,
        .name =
            "average",
    },
    {
        .name = "avg", // alias
        .value = RRDR_GROUPING_AVERAGE,
    },
    {
        .name = "sum",
        .other = nested_call((struct thing){ .value = 7 }),
        .value = RRDR_GROUPING_SUM,
    },
    { .name = NULL, .value = RRDR_GROUPING_UNDEFINED },
};
`)

	got, err := parseGroupingRoster(enumSource, registrySource)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"RRDR_GROUPING_AVERAGE", "RRDR_GROUPING_SUM"}; !reflect.DeepEqual(got.Order, want) {
		t.Fatalf("Order = %v, want %v", got.Order, want)
	}
	if got.Canonical["RRDR_GROUPING_AVERAGE"] != "average" ||
		!reflect.DeepEqual(got.Aliases["RRDR_GROUPING_AVERAGE"], []string{"avg"}) ||
		got.Canonical["RRDR_GROUPING_SUM"] != "sum" {
		t.Fatalf("parsed roster = %+v", got)
	}
}

func TestGroupingRosterParserRejectsDrift(t *testing.T) {
	validEnum := `
typedef enum rrdr_time_grouping {
    RRDR_GROUPING_UNDEFINED,
    RRDR_GROUPING_AVERAGE,
    RRDR_GROUPING_SUM,
    RRDR_GROUPING_SENTINEL,
} RRDR_TIME_GROUPING;
`
	validRegistry := `
static struct grouping api_v1_data_groups[] = {
    { .name = "average", .value = RRDR_GROUPING_AVERAGE },
    { .name = "sum", .value = RRDR_GROUPING_SUM },
};
`

	tests := map[string]struct {
		enum, registry string
		want           string
	}{
		"duplicate enum": {
			enum: `
typedef enum rrdr_time_grouping {
    RRDR_GROUPING_UNDEFINED,
    RRDR_GROUPING_AVERAGE,
    RRDR_GROUPING_AVERAGE,
    RRDR_GROUPING_SUM,
    RRDR_GROUPING_SENTINEL,
} RRDR_TIME_GROUPING;
`,
			registry: validRegistry,
			want:     "declared twice",
		},
		"unknown registry value": {
			enum: validEnum,
			registry: `
static struct grouping api_v1_data_groups[] = {
    { .name = "average", .value = RRDR_GROUPING_AVERAGE },
    { .name = "sum", .value = RRDR_GROUPING_SUM },
    { .name = "ghost", .value = RRDR_GROUPING_GHOST },
};
`,
			want: "does not declare",
		},
		"unnamed enum member": {
			enum: validEnum,
			registry: `
static struct grouping api_v1_data_groups[] = {
    { .name = "average", .value = RRDR_GROUPING_AVERAGE },
};
`,
			want: "no name",
		},
		"duplicate public name": {
			enum: validEnum,
			registry: `
static struct grouping api_v1_data_groups[] = {
    { .name = "same", .value = RRDR_GROUPING_AVERAGE },
    { .name = "same", .value = RRDR_GROUPING_SUM },
};
`,
			want: "offered twice",
		},
		"name without value": {
			enum: validEnum,
			registry: `
static struct grouping api_v1_data_groups[] = {
    { .name = "average" },
    { .name = "sum", .value = RRDR_GROUPING_SUM },
};
`,
			want: "no grouping value",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseGroupingRoster([]byte(tc.enum), []byte(tc.registry))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestDirectBraceEntriesRejectsUnclosedEntry(t *testing.T) {
	tests := map[string]string{
		"top-level": `{ .name = "average" } {`,
		"nested": `{ .name = "average" } {
			.other = nested_call((struct thing){ .value = 7 }`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			entries, err := directBraceEntries(source)
			if err == nil || !strings.Contains(err.Error(), "closing brace not found") {
				t.Fatalf("error = %v, want closing-brace error", err)
			}
			if entries != nil {
				t.Fatalf("entries = %q, want nil on malformed input", entries)
			}
		})
	}
}
