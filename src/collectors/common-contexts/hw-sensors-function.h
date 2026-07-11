// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HW_SENSORS_FUNCTION_H
#define NETDATA_HW_SENSORS_FUNCTION_H

// --------------------------------------------------------------------------------------------------------------------
// Shared schema of the cross-OS "sensors" function
//
// Every hardware-sensor producer exposes a "sensors" function with the SAME
// table schema, so the UI experience is identical on every operating system.
// This header is self-contained (libnetdata BUFFER APIs only), so it is
// usable both by in-daemon producers (macos.plugin, windows.plugin) and by
// external PLUGINSD plugins (debugfs.plugin).
//
// Producers emit one data row per sensor, with the values in EXACTLY this
// order (matching hw_sensors_function_columns() below):
//
//   1. Chart      - the per-sensor chart id (unique key, sticky)
//   2. Label      - human-readable sensor label
//   3. Kind       - temperature/voltage/current/power/fan/...
//   4. Subsystem  - hardware subsystem (cpu, soc, memory, storage, ...)
//   5. Source     - discovery source (smc, iohid, libsensors, ...)
//   6. Device     - device / chip identifier
//   7. Sensor     - producer-specific sensor identifier
//   8. Reading    - current reading (double; NAN when not available)
//   9. Units      - units of the reading
//  10. State      - sensor/collection state (ok, missing, alarm, ...)
//  11. Charts     - charting mode ("per-sensor" or "summary")
//  12. Count      - always 1 (powers grouped sensor-count tiles)
//  13. Temperature- Reading when Kind is temperature, else NAN
//  14. Fan        - Reading when Kind is fan, else NAN
//  15. Voltage    - Reading when Kind is voltage, else NAN
//  16. Current    - Reading when Kind is current, else NAN
//  17. Power      - Reading when Kind is power, else NAN
//
// The registration-follows-discovery rule also applies to every producer:
// the function is only registered/declared once at least one sensor exists.

static inline void hw_sensors_function_columns(BUFFER *wb) {
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "Chart", "Per-Sensor Chart ID",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Label", "Sensor Label",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Kind", "Sensor Kind",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Subsystem", "Hardware Subsystem",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Source", "Discovery Source",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Device", "Device / Chip",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Sensor", "Sensor Identifier",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Reading", "Current Reading",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                3, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Units", "Reading Units",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "State", "Sensor State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Charts", "Charting Mode",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Count", "Sensors Count",
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                0, "sensors", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Temperature", "Temperature",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "degrees Celsius", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Fan", "Fan Speed",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                0, "rpm", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Voltage", "Voltage",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                3, "V", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Current", "Current",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                3, "A", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Power", "Power",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "W", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
    }
    buffer_json_object_close(wb); // columns
}

static inline void hw_sensors_function_presentation(BUFFER *wb) {
    buffer_json_member_add_string(wb, "default_sort_column", "Label");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Sensors");
        {
            buffer_json_member_add_string(wb, "name", "Sensors");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Count");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        static const char *kind_charts[] = {"Temperature", "Fan", "Voltage", "Current", "Power"};
        for (size_t i = 0; i < sizeof(kind_charts) / sizeof(kind_charts[0]); i++) {
            buffer_json_member_add_object(wb, kind_charts[i]);
            {
                buffer_json_member_add_string(wb, "name", kind_charts[i]);
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, kind_charts[i]);
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Sensors");
        buffer_json_add_array_item_string(wb, "Subsystem");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Temperature");
        buffer_json_add_array_item_string(wb, "Subsystem");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        static const struct {
            const char *id;
            const char *name;
        } groups[] = {
            {"Kind", "Sensors by Kind"},
            {"Subsystem", "Sensors by Subsystem"},
            {"Source", "Sensors by Discovery Source"},
            {"Device", "Sensors by Device"},
            {"State", "Sensors by State"},
        };

        for (size_t i = 0; i < sizeof(groups) / sizeof(groups[0]); i++) {
            buffer_json_member_add_object(wb, groups[i].id);
            {
                buffer_json_member_add_string(wb, "name", groups[i].name);
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, groups[i].id);
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }
    }
    buffer_json_object_close(wb); // group_by
}

#endif //NETDATA_HW_SENSORS_FUNCTION_H
