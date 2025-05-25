// SPDX-License-Identifier: GPL-3.0-or-later

#include "json-keys.h"

// Static key name structures
static const JSON_KEY_NAMES json_short_keys = {
    .selected = "sl",
    .excluded = "ex",
    .queried = "qr",
    .failed = "fl",
    .dimensions = "ds",
    .instances = "is",
    .alerts = "al",
    .statistics = "sts",
    .name = "nm",
    .hostname = "nm",
    .node_id = "nd",
    .value = "vl",
    .label_values = "vl",
    .machine_guid = "mg",
    .agent_index = "ai",
    .clear = "cl",
    .warning = "wr",
    .critical = "cr",
    .other = "ot",
    .count = "cnt",
    .volume = "vol",
    .anomaly_rate = "arp",
    .anomaly_count = "arc",
    .contribution = "con",
    .point_annotations = "pa",
    .point_schema = "point",
    .priority = "pri",
    .update_every = "ue",
    .tier = "tr",
    .after = "af",
    .before = "bf",
    .status = "st",
    .first_entry = "fe",
    .last_entry = "le",
    .node_index = "ni",
    .units = "un",
    .weight = "wg",
};

static const JSON_KEY_NAMES json_long_keys = {
    .selected = "selected",
    .excluded = "excluded",
    .queried = "queried",
    .failed = "failed",
    .dimensions = "dimensions",
    .instances = "instances",
    .alerts = "alerts",
    .statistics = "statistics",
    .name = "name",
    .hostname = "hostname",
    .node_id = "node_id",
    .value = "value",
    .label_values = "label_values",
    .machine_guid = "machine_guid",
    .agent_index = "agents_array_index",
    .clear = "clear",
    .warning = "warning",
    .critical = "critical",
    .other = "other",
    .count = "count",
    .volume = "volume",
    .anomaly_rate = "anomaly_rate_percent",
    .anomaly_count = "anomalous_points_count",
    .contribution = "contribution_percent",
    .point_annotations = "point_annotations_bitmap",
    .point_schema = "point_schema",
    .priority = "priority",
    .update_every = "update_every",
    .tier = "tier",
    .after = "after",
    .before = "before",
    .status = "status",
    .first_entry = "first_entry",
    .last_entry = "last_entry",
    .node_index = "nodes_array_index",
    .units = "units",
    .weight = "weight",
};

// Thread-local pointer to the current key names
__thread const JSON_KEY_NAMES *json_keys = &json_short_keys;

void json_keys_init(JSON_KEYS_OPTIONS options) {
    if(options & JSON_KEYS_OPTION_LONG_KEYS)
        json_keys = &json_long_keys;
    else
        json_keys = &json_short_keys;
}

void json_keys_reset(void) {
    json_keys = &json_short_keys;
}

bool json_keys_are_long(void) {
    return json_keys == &json_long_keys;
}