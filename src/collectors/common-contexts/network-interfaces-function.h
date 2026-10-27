// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_INTERFACES_FUNCTION_H
#define NETDATA_NETWORK_INTERFACES_FUNCTION_H

// Shared presentation for the cross-platform network-interfaces Function.
// Row producers must emit values in the order of these columns and use NAN
// for unavailable numeric readings.
typedef struct {
    double traffic_rx;
    double traffic_tx;
    double traffic;
    double packets_rx;
    double packets_tx;
    double mcast_rx;
    double drops_rx;
    double drops_tx;
} network_interfaces_function_maxima;

static inline void network_interfaces_function_schema(BUFFER *wb, const network_interfaces_function_maxima *maxima) {
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "Interface", "Network Interface Name",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Type", "Network Interface Type",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "UsedBy", "Indicates whether the network interface is used by a cgroup or by the host system",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "PhState", "Current Physical State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OpState", "Current Operational State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Duplex", "Current Duplex Mode",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Speed", "Current Link Speed",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                0, "Mbit", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MTU", "Maximum Transmission Unit",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                0, "Octets", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "In", "Traffic Received",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Mbit", maxima->traffic_rx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Out", "Traffic Sent",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Mbit", maxima->traffic_tx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Total", "Traffic Received and Sent",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Mbit", maxima->traffic, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "PktsIn", "Received Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Kpps", maxima->packets_rx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "PktsOut", "Sent Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Kpps", maxima->packets_tx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "McastIn", "Multicast Received Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Kpps", maxima->mcast_rx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DropsIn", "Dropped Inbound Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Drops", maxima->drops_rx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DropsOut", "Dropped Outbound Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Drops", maxima->drops_tx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(
                wb, field_id++,
                "rowOptions", "rowOptions",
                RRDF_FIELD_TYPE_NONE,
                RRDR_FIELD_VISUAL_ROW_OPTIONS,
                RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                RRDF_FIELD_SORT_FIXED,
                NULL,
                RRDF_FIELD_SUMMARY_COUNT,
                RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_DUMMY,
                NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Total");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Traffic");
        {
            buffer_json_member_add_string(wb, "name", "Traffic");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "In");
                buffer_json_add_array_item_string(wb, "Out");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Packets");
        {
            buffer_json_member_add_string(wb, "name", "Packets");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "PktsIn");
                buffer_json_add_array_item_string(wb, "PktsOut");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Traffic");
        buffer_json_add_array_item_string(wb, "Interface");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Traffic");
        buffer_json_add_array_item_string(wb, "Type");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "Type");
        {
            buffer_json_member_add_string(wb, "name", "Type");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Type");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "UsedBy");
        {
            buffer_json_member_add_string(wb, "name", "UsedBy");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "UsedBy");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by
}

#endif // NETDATA_NETWORK_INTERFACES_FUNCTION_H
