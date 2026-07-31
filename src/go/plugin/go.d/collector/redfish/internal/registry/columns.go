// SPDX-License-Identifier: GPL-3.0-or-later

package registry

func column(order int, id string, typ ColumnType, visible, facet bool) ColumnSpec {
	return ColumnSpec{
		Order:    order,
		ID:       id,
		Name:     id,
		Tooltip:  id,
		Type:     typ,
		Visible:  visible,
		Facet:    facet,
		Sortable: true,
	}
}

func commonColumns() []ColumnSpec {
	columns := []ColumnSpec{
		column(0, "row_key", ColumnString, false, false),
		column(1, "sort_key", ColumnString, false, false),
		column(2, "row_type", ColumnEnum, true, true),
		column(3, "reading_key", ColumnString, false, false),
		column(4, "severity", ColumnEnum, true, true),
		column(5, "severity_rank", ColumnInteger, false, true),
		column(6, "observed_at", ColumnTimestamp, true, true),
		column(7, "membership_complete", ColumnBoolean, true, true),
		column(8, "acquisition_state", ColumnEnum, true, true),
		column(9, "error_class", ColumnEnum, false, true),
		column(10, "endpoint_job", ColumnString, true, true),
		column(11, "endpoint_key", ColumnString, false, true),
		column(12, "host_uri", ColumnString, true, true),
		column(13, "host_key", ColumnString, false, true),
		column(14, "host_name", ColumnString, true, true),
		column(15, "resource_kind", ColumnEnum, true, true),
		column(16, "resource_key", ColumnString, false, true),
		column(17, "id", ColumnString, true, true),
		column(18, "name", ColumnString, true, true),
		column(19, "resource_uri", ColumnString, true, true),
		column(20, "identity_quality", ColumnEnum, false, true),
		column(21, "source_container_uri", ColumnString, false, true),
		column(22, "source_property_path", ColumnString, false, true),
		column(23, "source_position", ColumnInteger, false, true),
		column(24, "rollup_owner_kind", ColumnEnum, false, true),
		column(25, "rollup_owner_key", ColumnString, false, true),
		column(26, "rollup_owner_name", ColumnString, false, true),
		column(27, "rollup_owner_uri", ColumnString, false, true),
		column(28, "logical_owner_key", ColumnString, false, true),
		column(29, "logical_owner_name", ColumnString, false, true),
		column(30, "logical_owner_uri", ColumnString, false, true),
		column(31, "logical_owner_candidates", ColumnString, false, true),
		column(32, "logical_owner_reason", ColumnEnum, false, true),
		column(33, "health", ColumnEnum, true, true),
		column(34, "health_rollup", ColumnEnum, true, true),
		column(35, "state", ColumnEnum, true, true),
		column(36, "power_state", ColumnEnum, true, true),
		column(37, "failure_predicted", ColumnBoolean, true, true),
		column(38, "condition_ok_count", ColumnInteger, true, true),
		column(39, "condition_warning_count", ColumnInteger, true, true),
		column(40, "condition_critical_count", ColumnInteger, true, true),
		column(41, "condition_unknown_count", ColumnInteger, true, true),
		column(42, "component_family", ColumnEnum, true, true),
		column(43, "detail_gate", ColumnEnum, false, true),
		column(44, "detail_component_count", ColumnInteger, false, true),
		column(45, "detail_component_cap", ColumnInteger, false, true),
		column(46, "source_schema", ColumnString, false, true),
		column(47, "source_uris", ColumnString, false, true),
		column(48, "source_models", ColumnString, false, true),
		column(49, "response_content_type_state", ColumnEnum, false, true),
		column(50, "response_odata_version_state", ColumnEnum, false, true),
		column(51, "description", ColumnString, false, false),
		column(52, "manufacturer", ColumnString, true, true),
		column(53, "model", ColumnString, true, true),
		column(54, "serial_number", ColumnString, true, true),
		column(55, "part_number", ColumnString, false, true),
		column(56, "spare_part_number", ColumnString, false, true),
		column(57, "sku", ColumnString, false, true),
		column(58, "asset_tag", ColumnString, true, true),
		column(59, "firmware_version", ColumnString, true, true),
		column(60, "uuid", ColumnString, false, true),
		column(61, "hot_pluggable", ColumnBoolean, false, true),
		column(62, "replaceable", ColumnBoolean, false, true),
		column(63, "ready_to_remove", ColumnBoolean, true, true),
		column(64, "location_indicator_active", ColumnBoolean, false, true),
		column(65, "location_service_label", ColumnString, true, true),
		column(66, "location_ordinal", ColumnInteger, false, true),
		column(67, "location_type", ColumnEnum, false, true),
		column(68, "location_orientation", ColumnEnum, false, true),
		column(69, "location_reference", ColumnEnum, false, true),
		column(70, "location_part_context", ColumnString, false, true),
		column(71, "location_rack", ColumnString, true, true),
		column(72, "location_rack_offset", ColumnInteger, false, true),
		column(73, "location_rack_offset_units", ColumnEnum, false, true),
		column(74, "physical_context", ColumnEnum, true, true),
		column(75, "physical_subcontext", ColumnEnum, false, true),
		column(76, "hardware_version", ColumnString, false, true),
	}
	columns[0].Unique = true
	for index := range columns {
		switch columns[index].ID {
		case "logical_owner_candidates", "source_uris", "source_models":
			columns[index].Structured = true
			columns[index].Sortable = false
			columns[index].Facet = false
		}
	}
	for _, index := range []int{0, 4, 15, 18} {
		columns[index].Sticky = true
	}
	return columns
}

func readingColumns(start int) []ColumnSpec {
	type declaration struct {
		id      string
		typ     ColumnType
		visible bool
		facet   bool
	}
	values := []declaration{
		{"reading_source_path", ColumnString, false, true},
		{"reading_source_type", ColumnEnum, true, true},
		{"reading_source_units", ColumnString, true, true},
		{"reading_source_basis", ColumnEnum, false, true},
		{"reading_source_value", ColumnFloat, true, true},
		{"reading_source_value_exact", ColumnString, false, false},
		{"reading_type", ColumnEnum, true, true},
		{"reading_units", ColumnString, true, true},
		{"reading_basis", ColumnEnum, true, true},
		{"reading_value", ColumnFloat, true, true},
		{"reading_data_source_uri", ColumnString, false, true},
		{"reading_time", ColumnTimestamp, false, true},
		{"reading_range_min_source", ColumnFloat, false, true},
		{"reading_range_max_source", ColumnFloat, false, true},
		{"reading_range_min", ColumnFloat, false, true},
		{"reading_range_max", ColumnFloat, false, true},
		{"reading_average_source", ColumnFloat, false, true},
		{"reading_average", ColumnFloat, false, true},
		{"reading_lowest_interval_source", ColumnFloat, false, true},
		{"reading_lowest_interval", ColumnFloat, false, true},
		{"reading_peak_interval_source", ColumnFloat, false, true},
		{"reading_peak_interval", ColumnFloat, false, true},
		{"reading_lowest_source", ColumnFloat, false, true},
		{"reading_lowest", ColumnFloat, false, true},
		{"reading_peak_source", ColumnFloat, false, true},
		{"reading_peak", ColumnFloat, false, true},
		{"minimum_allowable_source", ColumnFloat, false, true},
		{"minimum_allowable", ColumnFloat, false, true},
		{"maximum_allowable_source", ColumnFloat, false, true},
		{"maximum_allowable", ColumnFloat, false, true},
		{"adjusted_minimum_allowable_source", ColumnFloat, false, true},
		{"adjusted_minimum_allowable", ColumnFloat, false, true},
		{"adjusted_maximum_allowable_source", ColumnFloat, false, true},
		{"adjusted_maximum_allowable", ColumnFloat, false, true},
		{"reading_accuracy_source", ColumnFloat, false, true},
		{"reading_accuracy", ColumnFloat, false, true},
		{"calibration_source", ColumnFloat, false, true},
		{"calibration", ColumnFloat, false, true},
		{"accuracy_percent", ColumnFloat, false, true},
		{"precision", ColumnFloat, false, true},
		{"averaging_interval", ColumnFloat, false, true},
		{"averaging_interval_achieved", ColumnBoolean, false, true},
		{"sensor_reset_time", ColumnTimestamp, false, true},
		{"lowest_reading_time", ColumnTimestamp, false, true},
		{"peak_reading_time", ColumnTimestamp, false, true},
		{"calibration_time", ColumnTimestamp, false, true},
		{"implementation_type", ColumnEnum, false, true},
		{"electrical_context", ColumnEnum, false, true},
		{"voltage_type", ColumnEnum, false, true},
		{"lifetime_reading_source", ColumnFloat, false, true},
		{"lifetime_start_datetime", ColumnTimestamp, false, true},
		{"source_alarm_state", ColumnEnum, true, true},
		{"derived_alarm_state", ColumnEnum, true, true},
		{"effective_alarm_state", ColumnEnum, true, true},
		{"effective_alarm_source", ColumnEnum, false, true},
		{"effective_alarm_reason", ColumnString, false, true},
	}
	result := make([]ColumnSpec, 0, len(values)+70)
	for _, value := range values {
		item := column(start+len(result), value.id, value.typ, value.visible, value.facet)
		item.Members = map[string]struct{}{"__reading__": {}}
		result = append(result, item)
	}
	for _, role := range []string{
		"lower_caution", "lower_caution_user", "lower_critical", "lower_critical_user", "lower_fatal",
		"upper_caution", "upper_caution_user", "upper_critical", "upper_critical_user", "upper_fatal",
	} {
		for _, suffix := range []struct {
			name string
			typ  ColumnType
		}{
			{"_source", ColumnFloat},
			{"", ColumnFloat},
			{"_activation", ColumnEnum},
			{"_dwell_seconds", ColumnFloat},
			{"_hysteresis_duration_seconds", ColumnFloat},
			{"_hysteresis_source", ColumnFloat},
			{"_hysteresis", ColumnFloat},
		} {
			item := column(
				start+len(result),
				"threshold_"+role+suffix.name,
				suffix.typ,
				false,
				true,
			)
			item.Members = map[string]struct{}{"__reading__": {}}
			result = append(result, item)
		}
	}
	return result
}
