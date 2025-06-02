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
    .contexts = "ctx",
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
    .error = "er",
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
    .summary = "sum",
    .instances_count = "in",
    .nodes_count = "nd",
    .configurations_count = "cfg",
    .instance_id = "ch",
    .instance_name = "ch_n",
    .family = "fami",
    .context = "ctx",

    .alerts_index_id = "ati",
    .alert_global_id = "gi",
    .alert_name = "nm",
    .last_transition_id = "tr_i",
    .last_transition_value = "tr_v",
    .last_transition_timestamp = "tr_t",
    .last_updated_value = "v",
    .last_updated_timestamp = "t",

    .classification = "cl",
    .classifications = "cls",
    .component = "cm",
    .components = "cp",
    .type = "tp",
    .types = "ty",
    .recipients = "to",
    .source = "src",
    .config_hash_id = "cfg",
};

static const JSON_KEY_NAMES json_long_keys = {
    .selected = "selected",
    .excluded = "excluded",
    .queried = "queried",
    .failed = "failed",
    .dimensions = "dimensions",
    .instances = "instances",
    .contexts = "contexts",
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
    .error = "error",
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
    .summary = "summary",
    .instances_count = "instances_count",
    .nodes_count = "nodes_count",
    .configurations_count = "configurations_count",
    .instance_id = "instance_id",
    .instance_name = "instance",
    .family = "family",
    .context = "context",

    .alerts_index_id = "alerts_array_index_id",
    .alert_global_id = "global_id",
    .alert_name = "alert",
    .last_transition_id = "last_transition_id",
    .last_transition_value = "last_transition_value",
    .last_transition_timestamp = "last_transition_timestamp",
    .last_updated_value = "last_updated_value",
    .last_updated_timestamp = "last_updated_timestamp",

    .classification = "classification",
    .classifications = "classifications",
    .component = "component",
    .components = "components",
    .type = "type",
    .types = "types",
    .recipients = "recipients",
    .source = "source",
    .config_hash_id = "config_hash_id",
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