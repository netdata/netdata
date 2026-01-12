// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_YAML_H
#define NETDATA_YAML_H

#include "../libnetdata.h"
#include <yaml.h>

#ifdef __cplusplus
extern "C" {
#endif

// Flags for YAML parsing behavior
typedef enum yaml_to_json_flags {
    YAML2JSON_DEFAULT = 0,
    YAML2JSON_ALL_VALUES_AS_STRINGS = (1 << 0),  // Parse all scalar values as strings (no type conversion)
} YAML2JSON_FLAGS;

// Parse YAML from various sources and convert to json-c object
struct json_object *yaml_parse_string(const char *yaml_string, BUFFER *error, YAML2JSON_FLAGS flags);
struct json_object *yaml_parse_filename(const char *filename, BUFFER *error, YAML2JSON_FLAGS flags);
struct json_object *yaml_parse_fd(int fd, BUFFER *error, YAML2JSON_FLAGS flags);

// Generate YAML from json-c object to various destinations
bool yaml_generate_to_buffer(BUFFER *dst, struct json_object *json, BUFFER *error);
bool yaml_generate_to_filename(const char *filename, struct json_object *json, BUFFER *error);
bool yaml_generate_to_fd(int fd, struct json_object *json, BUFFER *error);

// Test functions
int yaml_unittest(void);
int yaml_comprehensive_unittest(void);

// Internal helper functions
struct json_object *yaml_document_to_json(yaml_document_t *document, BUFFER *error);
bool json_to_yaml_document(struct json_object *json, yaml_document_t *document, BUFFER *error);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_YAML_H