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

typedef struct {
    const char *id;
    const char *name;
    RRDF_FIELD_TYPE type;
    RRDF_FIELD_VISUAL visual;
    RRDF_FIELD_TRANSFORM transform;
    size_t decimal_points;
    const char *units;
    double max;
    RRDF_FIELD_SORT sort;
    RRDF_FIELD_SUMMARY summary;
    RRDF_FIELD_FILTER filter;
    RRDF_FIELD_OPTIONS options;
} network_interfaces_function_column;

#define NETWORK_INTERFACES_STRING_COLUMN(id, name, options)                                                     \
    { id, name, RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,       \
      RRDF_FIELD_SORT_ASCENDING, RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT, options }

#define NETWORK_INTERFACES_VALUE_COLUMN(id, name, units)                                                        \
    { id, name, RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0,      \
      units, NAN, RRDF_FIELD_SORT_DESCENDING, RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,          \
      RRDF_FIELD_OPTS_UNIQUE_KEY }

#define NETWORK_INTERFACES_RATE_COLUMN(id, name, units, max, options)                                           \
    { id, name, RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER, 2,        \
      units, max, RRDF_FIELD_SORT_DESCENDING, RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE, options }

static inline void network_interfaces_function_columns(BUFFER *wb, const network_interfaces_function_maxima *maxima) {
    const network_interfaces_function_column columns[] = {
        NETWORK_INTERFACES_STRING_COLUMN("Interface", "Network Interface Name",
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY),
        NETWORK_INTERFACES_STRING_COLUMN("Type", "Network Interface Type",
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY),
        NETWORK_INTERFACES_STRING_COLUMN("UsedBy", "Indicates whether the network interface is used by a cgroup or by the host system",
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY),
        NETWORK_INTERFACES_STRING_COLUMN("PhState", "Current Physical State",
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY),
        NETWORK_INTERFACES_STRING_COLUMN("OpState", "Current Operational State", RRDF_FIELD_OPTS_UNIQUE_KEY),
        NETWORK_INTERFACES_STRING_COLUMN("Duplex", "Current Duplex Mode", RRDF_FIELD_OPTS_UNIQUE_KEY),
        NETWORK_INTERFACES_VALUE_COLUMN("Speed", "Current Link Speed", "Mbit"),
        NETWORK_INTERFACES_VALUE_COLUMN("MTU", "Maximum Transmission Unit", "Octets"),
        NETWORK_INTERFACES_RATE_COLUMN("In", "Traffic Received", "Mbit", maxima->traffic_rx, RRDF_FIELD_OPTS_VISIBLE),
        NETWORK_INTERFACES_RATE_COLUMN("Out", "Traffic Sent", "Mbit", maxima->traffic_tx, RRDF_FIELD_OPTS_VISIBLE),
        NETWORK_INTERFACES_RATE_COLUMN("Total", "Traffic Received and Sent", "Mbit", maxima->traffic, RRDF_FIELD_OPTS_NONE),
        NETWORK_INTERFACES_RATE_COLUMN("PktsIn", "Received Packets", "Kpps", maxima->packets_rx, RRDF_FIELD_OPTS_VISIBLE),
        NETWORK_INTERFACES_RATE_COLUMN("PktsOut", "Sent Packets", "Kpps", maxima->packets_tx, RRDF_FIELD_OPTS_VISIBLE),
        NETWORK_INTERFACES_RATE_COLUMN("McastIn", "Multicast Received Packets", "Kpps", maxima->mcast_rx, RRDF_FIELD_OPTS_NONE),
        NETWORK_INTERFACES_RATE_COLUMN("DropsIn", "Dropped Inbound Packets", "Drops", maxima->drops_rx, RRDF_FIELD_OPTS_VISIBLE),
        NETWORK_INTERFACES_RATE_COLUMN("DropsOut", "Dropped Outbound Packets", "Drops", maxima->drops_tx, RRDF_FIELD_OPTS_VISIBLE),
        { "rowOptions", "rowOptions", RRDF_FIELD_TYPE_NONE, RRDR_FIELD_VISUAL_ROW_OPTIONS, RRDF_FIELD_TRANSFORM_NONE,
          0, NULL, NAN, RRDF_FIELD_SORT_FIXED, RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE, RRDF_FIELD_OPTS_DUMMY },
    };

    buffer_json_member_add_object(wb, "columns");

    for (size_t i = 0; i < sizeof(columns) / sizeof(columns[0]); i++) {
        const network_interfaces_function_column *c = &columns[i];
        buffer_rrdf_table_add_field(wb, i, c->id, c->name,
                c->type, c->visual, c->transform,
                c->decimal_points, c->units, c->max, c->sort, NULL,
                c->summary, c->filter, c->options, NULL);
    }

    buffer_json_object_close(wb); // columns
}

#undef NETWORK_INTERFACES_STRING_COLUMN
#undef NETWORK_INTERFACES_VALUE_COLUMN
#undef NETWORK_INTERFACES_RATE_COLUMN

static inline void network_interfaces_function_presentation(BUFFER *wb) {
    buffer_json_member_add_string(wb, "default_sort_column", "Total");

    buffer_json_member_add_object(wb, "charts");
    {
        static const struct {
            const char *id;
            const char *columns[2];
        } charts[] = {
            {"Traffic", {"In", "Out"}},
            {"Packets", {"PktsIn", "PktsOut"}},
        };

        for (size_t i = 0; i < sizeof(charts) / sizeof(charts[0]); i++) {
            buffer_json_member_add_object(wb, charts[i].id);
            {
                buffer_json_member_add_string(wb, "name", charts[i].id);
                buffer_json_member_add_string(wb, "type", "stacked-bar");
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, charts[i].columns[0]);
                    buffer_json_add_array_item_string(wb, charts[i].columns[1]);
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        static const char *group_columns[] = {"Interface", "Type"};

        for (size_t i = 0; i < sizeof(group_columns) / sizeof(group_columns[0]); i++) {
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, "Traffic");
            buffer_json_add_array_item_string(wb, group_columns[i]);
            buffer_json_array_close(wb);
        }
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        static const char *groups[] = {"Type", "UsedBy"};

        for (size_t i = 0; i < sizeof(groups) / sizeof(groups[0]); i++) {
            buffer_json_member_add_object(wb, groups[i]);
            {
                buffer_json_member_add_string(wb, "name", groups[i]);
                buffer_json_member_add_array(wb, "columns");
                {
                    buffer_json_add_array_item_string(wb, groups[i]);
                }
                buffer_json_array_close(wb);
            }
            buffer_json_object_close(wb);
        }
    }
    buffer_json_object_close(wb); // group_by
}

static inline void network_interfaces_function_schema(BUFFER *wb, const network_interfaces_function_maxima *maxima) {
    network_interfaces_function_columns(wb, maxima);
    network_interfaces_function_presentation(wb);
}

#endif // NETDATA_NETWORK_INTERFACES_FUNCTION_H
