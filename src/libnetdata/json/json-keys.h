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
    const char *alerts;             // "al" / "alerts"
    const char *statistics;         // "sts" / "statistics"
    
    // Common fields
    const char *name;               // "nm" / "name"
    const char *hostname;           // "nm" / "hostname"
    const char *node_id;            // "nd" / "node_id"
    const char *value;              // "vl" / "value"
    const char *label_values;       // "vl" / "label_values"
    const char *machine_guid;       // "mg" / "machine_guid"
    const char *agent_index;        // "ai" / "agents_array_index"
    
    // Alert levels
    const char *clear;              // "cl" / "clear"
    const char *warning;            // "wr" / "warning"
    const char *critical;           // "cr" / "critical"
    const char *other;              // "ot" / "other"
    
    // Statistics fields
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
    const char *node_index;         // "ni" / "nodes_array_index"
    const char *units;              // "un" / "units"
    const char *weight;             // "wg" / "weight"
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