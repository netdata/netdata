// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JSON_KEYS_H
#define NETDATA_JSON_KEYS_H

#include "../libnetdata.h"

// JSON key names struct for short vs long key support
typedef struct json_key_names {
    // Status and statistics
    const char *selected;           // "sl" / "selected"
    const char *excluded;           // "ex" / "excluded"
    const char *queried;            // "qr" / "queried"
    const char *failed;             // "fl" / "failed"
    
    // Object types
    const char *dimensions;         // "ds" / "dimensions"
    const char *instances;          // "is" / "instances"
    const char *contexts;           // "ctx" / "contexts"
    const char *alerts;             // "al" / "alerts"
    const char *statistics;         // "sts" / "statistics"
    
    const char *node_id;            // "nd" / "node_id"
    const char *machine_guid;       // "mg" / "machine_guid"

    // metadata by name
    const char *name;               // "nm" / "name"
    const char *hostname;           // "nm" / "hostname"
    const char *alert_name;         // "nm" / "alert_name"
    const char *context;            // "ctx" / "context"
    const char *instance_id;        // "ch" / "instance_id"
    const char *instance_name;      // "ch_n" / "instance"
    const char *family;             // "fami" / "family"

    // Values
    const char *value;              // "vl" / "value"
    const char *label_values;       // "vl" / "label_values"

    // Alert levels
    const char *clear;              // "cl" / "clear"
    const char *warning;            // "wr" / "warning"
    const char *critical;           // "cr" / "critical"
    const char *error;              // "er" / "error"
    const char *other;              // "ot" / "other"
    
    // Storage Points
    const char *count;              // "cnt" / "count"
    const char *volume;             // "vol" / "volume"
    const char *anomaly_rate;       // "arp" / "anomaly_rate_percent"
    const char *anomaly_count;      // "arc" / "anomalous_points_count"
    const char *contribution;       // "con" / "contribution_percent"
    const char *point_annotations;  // "pa" / "point_annotations_bitmap"
    const char *point_schema;       // "point" / "point_schema"
    
    // Other fields
    const char *priority;           // "pri" / "priority"
    const char *update_every;       // "ue" / "update_every"
    const char *tier;               // "tr" / "tier"
    const char *after;              // "af" / "after"
    const char *before;             // "bf" / "before"
    const char *status;             // "st" / "status"
    const char *first_entry;        // "fe" / "first_entry"
    const char *last_entry;         // "le" / "last_entry"
    const char *units;              // "un" / "units"
    const char *weight;             // "wg" / "weight"

    // indexes
    const char *agent_index;        // "ai" / "agents_array_index"
    const char *node_index;         // "ni" / "nodes_array_index"
    const char *alerts_index_id;    // "ati" / "alerts_array_index_id"

    // alerts fields
    const char *summary;                    // "sum" / "summary"
    const char *nodes_count;                // "nd" / "nodes_count"
    const char *instances_count;            // "in" / "instances_count"
    const char *configurations_count;       // "cfg" / "configurations_count"

    const char *alert_global_id;            // "gi" / "global_id"
    const char *last_transition_id;         // "tr_i" / "last_transition_id"
    const char *last_transition_value;      // "tr_v" / "last_transition_value"
    const char *last_transition_timestamp;  // "tr_t" / "last_transition_timestamp"
    const char *last_updated_value;         // "v" / "last_updated_value"
    const char *last_updated_timestamp;     // "t" / "last_updated_timestamp"

    const char *classification;             // "cl" / "classification"
    const char *classifications;            // "cls" / "classifications"
    const char *component;                  // "cm" / "component"
    const char *components;                 // "cp" / "components"
    const char *type;                       // "tp" / "type"
    const char *types;                      // "ty" / "types"
    const char *recipients;                 // "to" / "recipients"

    const char *source;                     // "src" / "source"
    const char *config_hash_id;         // "cfg" / "configuration_hash"
} JSON_KEY_NAMES;

// Options type for controlling key format
typedef enum {
    JSON_KEYS_OPTION_LONG_KEYS = (1 << 0),
} JSON_KEYS_OPTIONS;

// Thread-local pointer to the current key names
extern __thread const JSON_KEY_NAMES *json_keys;

// Macro for clean key access
#define JSKEY(member) (json_keys->member)

// Key name initialization
void json_keys_init(JSON_KEYS_OPTIONS options);
void json_keys_reset(void);

// Check if long keys are enabled
bool json_keys_are_long(void);

#endif //NETDATA_JSON_KEYS_H