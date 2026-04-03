// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const licensesMethodID = "licenses"

func licensesMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:          licensesMethodID,
		Name:        "Licenses",
		UpdateEvery: 10,
		Help:        "Normalized SNMP licensing rows for the selected device",
	}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcLicenses)(nil)

type funcLicenses struct {
	router *funcRouter
}

func newFuncLicenses(r *funcRouter) *funcLicenses {
	return &funcLicenses{router: r}
}

func (f *funcLicenses) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != licensesMethodID {
		return nil, nil
	}
	return nil, nil
}

func (f *funcLicenses) Cleanup(_ context.Context) {}

func (f *funcLicenses) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != licensesMethodID {
		return funcapi.NotFoundResponse(method)
	}

	if f.router.licenseCache == nil {
		return funcapi.UnavailableResponse("license data not available yet, please retry after data collection")
	}

	lastUpdate, rows := f.router.licenseCache.snapshot()
	if lastUpdate.IsZero() {
		return funcapi.UnavailableResponse("license data not available yet, please retry after data collection")
	}

	sortLicenseRows(rows)
	now := time.Now().UTC()
	cs := licenseColumnSet(licenseAllColumns)
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, buildLicenseFunctionRow(row, now))
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              "Normalized SNMP licensing rows for the selected device",
		Columns:           buildLicenseColumns(cs),
		Data:              data,
		DefaultSortColumn: defaultLicenseSortColumn(),
	}
}

type licenseColumn struct {
	funcapi.ColumnMeta
	Value       func(licenseRow, time.Time) any
	DefaultSort bool
}

func licenseColumnSet(cols []licenseColumn) funcapi.ColumnSet[licenseColumn] {
	return funcapi.Columns(cols, func(c licenseColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var licenseAllColumns = []licenseColumn{
	{ColumnMeta: funcapi.ColumnMeta{Name: "License", Tooltip: "License", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sticky: true, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return firstNonEmpty(r.Name, r.ID) }, DefaultSort: true},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Bucket", Tooltip: "Normalized State Bucket", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true, Visualization: funcapi.FieldVisualPill}, Value: func(r licenseRow, _ time.Time) any { return string(r.StateBucket) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "State", Tooltip: "Raw vendor state", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return emptyToNil(r.StateRaw) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Source", Tooltip: "Profile source", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return emptyToNil(r.Source) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "ID", Tooltip: "Stable row identifier", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true, UniqueKey: true}, Value: func(r licenseRow, _ time.Time) any { return licenseRowUniqueKey(r) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Feature", Tooltip: "Feature name", Type: funcapi.FieldTypeString, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return emptyToNil(r.Feature) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Component", Tooltip: "License component", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return emptyToNil(r.Component) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Type", Tooltip: "License type", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return emptyToNil(r.Type) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Remaining", Tooltip: "Time remaining until expiry", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true, Transform: funcapi.FieldTransformDuration}, Value: func(r licenseRow, ts time.Time) any {
		return licenseRemainingCell(r.ExpiryTS, r.HasExpiry && !r.IsPerpetual, ts)
	}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Expiry", Tooltip: "Expiry time", Type: funcapi.FieldTypeTimestamp, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true, Transform: funcapi.FieldTransformDatetime}, Value: func(r licenseRow, _ time.Time) any {
		return licenseTimestampCell(r.ExpiryTS, r.HasExpiry && !r.IsPerpetual)
	}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Auth Remaining", Tooltip: "Authorization time remaining", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true, Transform: funcapi.FieldTransformDuration}, Value: func(r licenseRow, ts time.Time) any {
		return licenseRemainingCell(r.AuthorizationExpiry, r.HasAuthorizationTime, ts)
	}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Auth Expiry", Tooltip: "Authorization expiry time", Type: funcapi.FieldTypeTimestamp, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true, Transform: funcapi.FieldTransformDatetime}, Value: func(r licenseRow, _ time.Time) any {
		return licenseTimestampCell(r.AuthorizationExpiry, r.HasAuthorizationTime)
	}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Cert Remaining", Tooltip: "Certificate time remaining", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true, Transform: funcapi.FieldTransformDuration}, Value: func(r licenseRow, ts time.Time) any {
		return licenseRemainingCell(r.CertificateExpiry, r.HasCertificateTime, ts)
	}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Cert Expiry", Tooltip: "Certificate expiry time", Type: funcapi.FieldTypeTimestamp, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true, Transform: funcapi.FieldTransformDatetime}, Value: func(r licenseRow, _ time.Time) any {
		return licenseTimestampCell(r.CertificateExpiry, r.HasCertificateTime)
	}},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Grace Remaining", Tooltip: "Grace/evaluation time remaining", Type: funcapi.FieldTypeDuration, Units: "milliseconds", Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true, Transform: funcapi.FieldTransformDuration}, Value: func(r licenseRow, ts time.Time) any { return licenseRemainingCell(r.GraceExpiry, r.HasGraceTime, ts) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Grace Expiry", Tooltip: "Grace/evaluation expiry time", Type: funcapi.FieldTypeTimestamp, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryMin, Filter: funcapi.FieldFilterRange, Sortable: true, Transform: funcapi.FieldTransformDatetime}, Value: func(r licenseRow, _ time.Time) any { return licenseTimestampCell(r.GraceExpiry, r.HasGraceTime) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Usage", Tooltip: "Used license units", Type: funcapi.FieldTypeInteger, Units: "licenses", Visible: true, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return licenseIntCell(r.Usage, r.HasUsage) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Capacity", Tooltip: "Total license capacity", Type: funcapi.FieldTypeInteger, Units: "licenses", Visible: true, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return licenseIntCell(r.Capacity, r.HasCapacity) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Available", Tooltip: "Available license units", Type: funcapi.FieldTypeInteger, Units: "licenses", Visible: false, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true}, Value: func(r licenseRow, _ time.Time) any { return licenseIntCell(r.Available, r.HasAvailable) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Usage %", Tooltip: "License usage percentage", Type: funcapi.FieldTypeFloat, Units: "percentage", Visible: true, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummaryMax, Filter: funcapi.FieldFilterRange, Sortable: true, Visualization: funcapi.FieldVisualBar, Transform: funcapi.FieldTransformNumber, DecimalPoints: 2}, Value: func(r licenseRow, _ time.Time) any { return licensePercentCell(r.UsagePercent, r.HasUsagePct) }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Unlimited", Tooltip: "Unlimited license pool", Type: funcapi.FieldTypeBoolean, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true, Visualization: funcapi.FieldVisualPill}, Value: func(r licenseRow, _ time.Time) any { return r.IsUnlimited }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Perpetual", Tooltip: "Perpetual license", Type: funcapi.FieldTypeBoolean, Visible: false, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true, Visualization: funcapi.FieldVisualPill}, Value: func(r licenseRow, _ time.Time) any { return r.IsPerpetual }},
	{ColumnMeta: funcapi.ColumnMeta{Name: "Impact", Tooltip: "Operational impact", Type: funcapi.FieldTypeString, Visible: true, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true, FullWidth: true, Wrap: true}, Value: func(r licenseRow, _ time.Time) any { return emptyToNil(r.Impact) }},
}

func buildLicenseColumns(cs funcapi.ColumnSet[licenseColumn]) map[string]any {
	columns := cs.BuildColumns()
	rowOptions := funcapi.Column{
		Index:         cs.Len(),
		Name:          "rowOptions",
		Type:          funcapi.FieldTypeNone,
		Visualization: funcapi.FieldVisualRowOptions,
		Sort:          funcapi.FieldSortAscending,
		Sortable:      false,
		Sticky:        false,
		Summary:       funcapi.FieldSummaryCount,
		Filter:        funcapi.FieldFilterNone,
		Visible:       false,
		Dummy:         true,
		ValueOptions: funcapi.ValueOptions{
			Transform:     funcapi.FieldTransformNone,
			DecimalPoints: 0,
			DefaultValue:  nil,
		},
	}
	columns["rowOptions"] = rowOptions.BuildColumn()
	return columns
}

func buildLicenseFunctionRow(row licenseRow, ts time.Time) []any {
	data := make([]any, len(licenseAllColumns)+1)
	for i, col := range licenseAllColumns {
		data[i] = col.Value(row, ts)
	}
	data[len(licenseAllColumns)] = nil
	return data
}

func defaultLicenseSortColumn() string {
	for _, col := range licenseAllColumns {
		if col.DefaultSort {
			return col.Name
		}
	}
	return "License"
}

func sortLicenseRows(rows []licenseRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]

		if lp, rp := licenseBucketPriority(left.StateBucket), licenseBucketPriority(right.StateBucket); lp != rp {
			return lp < rp
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.ID < right.ID
	})
}

func licenseBucketPriority(bucket licenseStateBucket) int {
	switch bucket {
	case licenseStateBucketBroken:
		return 0
	case licenseStateBucketDegraded:
		return 1
	case licenseStateBucketHealthy:
		return 2
	default:
		return 3
	}
}

func licenseRowUniqueKey(row licenseRow) string {
	return strings.Join([]string{
		row.Source,
		row.ID,
		row.Name,
		row.Feature,
		row.Component,
		row.Type,
		row.OriginalMetric,
	}, "|")
}

func licenseRemainingCell(expiry int64, ok bool, ts time.Time) any {
	if !ok {
		return nil
	}
	return (expiry - ts.Unix()) * 1000
}

func licenseTimestampCell(expiry int64, ok bool) any {
	if !ok {
		return nil
	}
	return time.Unix(expiry, 0).UnixMilli()
}

func licenseIntCell(value int64, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func licensePercentCell(value float64, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func emptyToNil(v string) any {
	if v == "" {
		return nil
	}
	return v
}
