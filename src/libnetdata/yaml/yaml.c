// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * Netdata YAML Parser/Generator Module
 * 
 * This module provides YAML parsing and generation using libyaml, with conversion
 * to/from json-c objects. It supports the YAML subset that is 100% compatible with JSON.
 * 
 * KNOWN LIMITATIONS (due to libyaml):
 * 1. Octal escape sequences (\101) are not supported - use hex (\x41) or unicode (\u0041)
 * 2. Single-quoted strings with literal newlines have them converted to spaces
 * 3. Null bytes in strings may cause issues
 * 4. Complex block scalar indentation may not be preserved exactly
 * 5. Some invalid YAML syntax may be accepted without error
 * 
 * The module handles these limitations gracefully and provides consistent behavior
 * for round-trip conversion where possible.
 */

#include "yaml.h"

static struct json_object *yaml_node_to_json(yaml_document_t *document, yaml_node_t *node, BUFFER *error, YAML2JSON_FLAGS flags);

static struct json_object *yaml_sequence_to_json(yaml_document_t *document, yaml_node_t *node, BUFFER *error, YAML2JSON_FLAGS flags) {
    struct json_object *array = json_object_new_array();
    if (!array) {
        buffer_strcat(error, "Failed to create JSON array");
        return NULL;
    }

    yaml_node_item_t *item;
    for (item = node->data.sequence.items.start; item < node->data.sequence.items.top; item++) {
        yaml_node_t *child = yaml_document_get_node(document, *item);
        if (!child) {
            buffer_sprintf(error, "Invalid sequence item reference");
            json_object_put(array);
            return NULL;
        }

        struct json_object *child_obj = yaml_node_to_json(document, child, error, flags);
        
        // Check for error (error buffer not empty)
        if (buffer_strlen(error) > 0) {
            // This is an error
            if (child_obj) json_object_put(child_obj);
            json_object_put(array);
            return NULL;
        }

        if (json_object_array_add(array, child_obj) != 0) {
            buffer_strcat(error, "Failed to add item to JSON array");
            json_object_put(child_obj);
            json_object_put(array);
            return NULL;
        }
    }

    return array;
}

static struct json_object *yaml_mapping_to_json(yaml_document_t *document, yaml_node_t *node, BUFFER *error, YAML2JSON_FLAGS flags) {
    struct json_object *object = json_object_new_object();
    if (!object) {
        buffer_strcat(error, "Failed to create JSON object");
        return NULL;
    }

    yaml_node_pair_t *pair;
    for (pair = node->data.mapping.pairs.start; pair < node->data.mapping.pairs.top; pair++) {
        yaml_node_t *key_node = yaml_document_get_node(document, pair->key);
        yaml_node_t *value_node = yaml_document_get_node(document, pair->value);

        if (!key_node || !value_node) {
            buffer_sprintf(error, "Invalid mapping pair reference");
            json_object_put(object);
            return NULL;
        }

        if (key_node->type != YAML_SCALAR_NODE) {
            buffer_sprintf(error, "Mapping key must be a scalar");
            json_object_put(object);
            return NULL;
        }

        const char *key = (const char *)key_node->data.scalar.value;
        struct json_object *value_obj = yaml_node_to_json(document, value_node, error, flags);
        
        // Check for error (error buffer not empty)
        if (buffer_strlen(error) > 0) {
            // This is an error
            if (value_obj) json_object_put(value_obj);
            json_object_put(object);
            return NULL;
        }

        // json_object_object_add handles NULL values correctly
        if (json_object_object_add(object, key, value_obj) != 0) {
            buffer_sprintf(error, "Failed to add property to JSON object");
            if (value_obj) json_object_put(value_obj);
            json_object_put(object);
            return NULL;
        }
    }

    return object;
}

// Helper function to parse numbers with underscores
static bool parse_number_with_underscores(const char *str, size_t len, long long *int_result, double *double_result, bool *is_double) {
    char cleaned[256];
    size_t j = 0;
    
    // Remove underscores
    for (size_t i = 0; i < len && j < sizeof(cleaned) - 1; i++) {
        if (str[i] != '_') {
            cleaned[j++] = str[i];
        }
    }
    cleaned[j] = '\0';
    
    char *endptr;
    errno = 0;
    
    // Try as integer first
    *int_result = strtoll(cleaned, &endptr, 10);
    if (errno == 0 && *endptr == '\0') {
        *is_double = false;
        return true;
    }
    
    // Try as double
    errno = 0;
    *double_result = strtod(cleaned, &endptr);
    if (errno == 0 && *endptr == '\0') {
        *is_double = true;
        return true;
    }
    
    return false;
}

static struct json_object *yaml_scalar_to_json(yaml_node_t *node, BUFFER *error, YAML2JSON_FLAGS flags) {
    const char *value = (const char *)node->data.scalar.value;
    size_t length = node->data.scalar.length;

    // If YAML2JSON_ALL_VALUES_AS_STRINGS flag is set, always return as string
    if (flags & YAML2JSON_ALL_VALUES_AS_STRINGS) {
        return json_object_new_string_len(value, (int)length);
    }

    // Check if this is a quoted or plain scalar
    bool is_quoted = (node->data.scalar.style == YAML_SINGLE_QUOTED_SCALAR_STYLE ||
                      node->data.scalar.style == YAML_DOUBLE_QUOTED_SCALAR_STYLE);

    // If quoted, always return as string
    if (is_quoted) {
        // LIBYAML LIMITATION: Single-quoted strings with literal newlines have them converted to spaces.
        // LIBYAML LIMITATION: Some escape sequences (like octal \101) are not supported.
        // LIBYAML LIMITATION: Null bytes (\0) may cause issues with string length.
        return json_object_new_string_len(value, (int)length);
    }

    // Handle null and tilde (case-insensitive for null)
    if ((length == 4 && strcasecmp(value, "null") == 0) ||
        (length == 1 && strncmp(value, "~", 1) == 0)) {
        // In json-c, NULL represents JSON null values
        return NULL;
    }

    // Handle booleans (case-insensitive)
    if ((length == 4 && strcasecmp(value, "true") == 0) ||
        (length == 3 && strcasecmp(value, "yes") == 0) ||
        (length == 2 && strcasecmp(value, "on") == 0)) {
        return json_object_new_boolean(1);
    }

    if ((length == 5 && strcasecmp(value, "false") == 0) ||
        (length == 2 && strcasecmp(value, "no") == 0) ||
        (length == 3 && strcasecmp(value, "off") == 0)) {
        return json_object_new_boolean(0);
    }

    // Try to parse as number
    char *endptr;
    errno = 0;

    // Check for hex (0x or 0X prefix)
    if (length > 2 && value[0] == '0' && (value[1] == 'x' || value[1] == 'X')) {
        // Check if it contains underscores
        bool has_underscore = false;
        for (size_t i = 2; i < length; i++) {
            if (value[i] == '_') {
                has_underscore = true;
                break;
            }
        }
        
        if (has_underscore) {
            // Remove underscores
            char cleaned[256];
            size_t j = 0;
            for (size_t i = 0; i < length && j < sizeof(cleaned) - 1; i++) {
                if (value[i] != '_') {
                    cleaned[j++] = value[i];
                }
            }
            cleaned[j] = '\0';
            
            long long int_val = strtoll(cleaned, &endptr, 16);
            if (errno == 0 && *endptr == '\0') {
                return json_object_new_int64(int_val);
            }
        } else {
            long long int_val = strtoll(value, &endptr, 16);
            if (errno == 0 && endptr == value + length) {
                return json_object_new_int64(int_val);
            }
        }
    }
    
    // Check for octal (0o or 0O prefix)
    if (length > 2 && value[0] == '0' && (value[1] == 'o' || value[1] == 'O')) {
        long long int_val = strtoll(value + 2, &endptr, 8);
        if (errno == 0 && endptr == value + length) {
            return json_object_new_int64(int_val);
        }
    }
    
    // Check for binary (0b or 0B prefix)
    if (length > 2 && value[0] == '0' && (value[1] == 'b' || value[1] == 'B')) {
        // Check if it contains underscores
        bool has_underscore = false;
        for (size_t i = 2; i < length; i++) {
            if (value[i] == '_') {
                has_underscore = true;
                break;
            }
        }
        
        if (has_underscore) {
            // Remove underscores
            char cleaned[256];
            size_t j = 0;
            for (size_t i = 0; i < length && j < sizeof(cleaned) - 1; i++) {
                if (value[i] != '_') {
                    cleaned[j++] = value[i];
                }
            }
            cleaned[j] = '\0';
            
            long long int_val = strtoll(cleaned + 2, &endptr, 2);
            if (errno == 0 && *endptr == '\0') {
                return json_object_new_int64(int_val);
            }
        } else {
            long long int_val = strtoll(value + 2, &endptr, 2);
            if (errno == 0 && endptr == value + length) {
                return json_object_new_int64(int_val);
            }
        }
    }

    // Check if number contains underscores
    bool has_underscore = false;
    for (size_t i = 0; i < length; i++) {
        if (value[i] == '_') {
            has_underscore = true;
            break;
        }
    }
    
    if (has_underscore) {
        long long int_result;
        double double_result;
        bool is_double;
        
        if (parse_number_with_underscores(value, length, &int_result, &double_result, &is_double)) {
            if (is_double) {
                return json_object_new_double(double_result);
            } else {
                return json_object_new_int64(int_result);
            }
        }
    }

    // Try integer
    long long int_val = strtoll(value, &endptr, 10);
    if (errno == 0 && endptr == value + length && *value != '\0') {
        return json_object_new_int64(int_val);
    }

    // Try double
    errno = 0;
    double double_val = strtod(value, &endptr);
    if (errno == 0 && endptr == value + length && *value != '\0') {
        return json_object_new_double(double_val);
    }

    // Default to string
    return json_object_new_string_len(value, (int)length);
}

static struct json_object *yaml_node_to_json(yaml_document_t *document, yaml_node_t *node, BUFFER *error, YAML2JSON_FLAGS flags) {
    if (!node) {
        buffer_strcat(error, "NULL node");
        return NULL;
    }

    switch (node->type) {
        case YAML_SCALAR_NODE:
            return yaml_scalar_to_json(node, error, flags);

        case YAML_SEQUENCE_NODE:
            return yaml_sequence_to_json(document, node, error, flags);

        case YAML_MAPPING_NODE:
            return yaml_mapping_to_json(document, node, error, flags);

        default:
            buffer_sprintf(error, "Unsupported YAML node type: %d", node->type);
            return NULL;
    }
}

struct json_object *yaml_document_to_json(yaml_document_t *document, BUFFER *error) {
    yaml_node_t *root = yaml_document_get_root_node(document);
    if (!root) {
        // Empty document - this is valid YAML but we return NULL
        return NULL;
    }

    return yaml_node_to_json(document, root, error, YAML2JSON_DEFAULT);
}

static struct json_object *yaml_document_to_json_with_flags(yaml_document_t *document, BUFFER *error, YAML2JSON_FLAGS flags) {
    yaml_node_t *root = yaml_document_get_root_node(document);
    if (!root) {
        // Empty document - this is valid YAML but we return NULL
        return NULL;
    }

    return yaml_node_to_json(document, root, error, flags);
}

static struct json_object *yaml_parse_common(yaml_parser_t *parser, BUFFER *error, YAML2JSON_FLAGS flags) {
    yaml_document_t document;
    struct json_object *result = NULL;

    if (!yaml_parser_load(parser, &document)) {
        if (parser->error == YAML_NO_ERROR) {
            buffer_strcat(error, "No YAML document found (empty input)");
        } else {
            buffer_sprintf(error, "YAML parse error: %s at line %zu, column %zu",
                          parser->problem ? parser->problem : "unknown error",
                          parser->problem_mark.line + 1,
                          parser->problem_mark.column + 1);
        }
        goto cleanup;
    }
    

    result = yaml_document_to_json_with_flags(&document, error, flags);
    yaml_document_delete(&document);

cleanup:
    yaml_parser_delete(parser);
    return result;
}

struct json_object *yaml_parse_string(const char *yaml_string, BUFFER *error, YAML2JSON_FLAGS flags) {
    if (!yaml_string) {
        buffer_strcat(error, "NULL YAML string");
        return NULL;
    }

    yaml_parser_t parser;
    if (!yaml_parser_initialize(&parser)) {
        buffer_strcat(error, "Failed to initialize YAML parser");
        return NULL;
    }

    yaml_parser_set_input_string(&parser, (const unsigned char *)yaml_string, strlen(yaml_string));
    return yaml_parse_common(&parser, error, flags);
}

struct json_object *yaml_parse_filename(const char *filename, BUFFER *error, YAML2JSON_FLAGS flags) {
    if (!filename) {
        buffer_strcat(error, "NULL filename");
        return NULL;
    }

    FILE *file = fopen(filename, "r");
    if (!file) {
        buffer_sprintf(error, "Failed to open file '%s': %s", filename, strerror(errno));
        return NULL;
    }

    yaml_parser_t parser;
    if (!yaml_parser_initialize(&parser)) {
        buffer_strcat(error, "Failed to initialize YAML parser");
        fclose(file);
        return NULL;
    }

    yaml_parser_set_input_file(&parser, file);
    struct json_object *result = yaml_parse_common(&parser, error, flags);
    fclose(file);
    return result;
}

struct json_object *yaml_parse_fd(int fd, BUFFER *error, YAML2JSON_FLAGS flags) {
    if (fd < 0) {
        buffer_strcat(error, "Invalid file descriptor");
        return NULL;
    }

    FILE *file = fdopen(dup(fd), "r");
    if (!file) {
        buffer_sprintf(error, "Failed to open file descriptor: %s", strerror(errno));
        return NULL;
    }

    yaml_parser_t parser;
    if (!yaml_parser_initialize(&parser)) {
        buffer_strcat(error, "Failed to initialize YAML parser");
        fclose(file);
        return NULL;
    }

    yaml_parser_set_input_file(&parser, file);
    struct json_object *result = yaml_parse_common(&parser, error, flags);
    fclose(file);
    return result;
}

// YAML generation functions

static int yaml_add_json_to_document(yaml_document_t *document, struct json_object *json, BUFFER *error);

static int yaml_add_array_to_document(yaml_document_t *document, struct json_object *array, BUFFER *error) {
    int sequence = yaml_document_add_sequence(document, NULL, YAML_BLOCK_SEQUENCE_STYLE);
    if (!sequence) {
        buffer_strcat(error, "Failed to add sequence to YAML document");
        return 0;
    }

    size_t len = json_object_array_length(array);
    for (size_t i = 0; i < len; i++) {
        struct json_object *item = json_object_array_get_idx(array, i);
        int item_node = yaml_add_json_to_document(document, item, error);
        if (!item_node) {
            return 0;
        }

        if (!yaml_document_append_sequence_item(document, sequence, item_node)) {
            buffer_strcat(error, "Failed to append item to YAML sequence");
            return 0;
        }
    }

    return sequence;
}

static int yaml_add_object_to_document(yaml_document_t *document, struct json_object *object, BUFFER *error) {
    int mapping = yaml_document_add_mapping(document, NULL, YAML_BLOCK_MAPPING_STYLE);
    if (!mapping) {
        buffer_strcat(error, "Failed to add mapping to YAML document");
        return 0;
    }

    json_object_object_foreach(object, key, value) {
        int key_node = yaml_document_add_scalar(document, NULL, 
                                               (yaml_char_t *)key, 
                                               strlen(key), 
                                               YAML_PLAIN_SCALAR_STYLE);
        if (!key_node) {
            buffer_sprintf(error, "Failed to add key '%s' to YAML document", key);
            return 0;
        }

        int value_node = yaml_add_json_to_document(document, value, error);
        if (!value_node) {
            return 0;
        }

        if (!yaml_document_append_mapping_pair(document, mapping, key_node, value_node)) {
            buffer_sprintf(error, "Failed to add mapping pair for key '%s'", key);
            return 0;
        }
    }

    return mapping;
}

static int yaml_add_json_to_document(yaml_document_t *document, struct json_object *json, BUFFER *error) {
    if (!json) {
        // This should not happen in normal usage, but handle it
        return yaml_document_add_scalar(document, NULL, 
                                      (yaml_char_t *)"null", 
                                      4, 
                                      YAML_PLAIN_SCALAR_STYLE);
    }

    enum json_type type = json_object_get_type(json);
    
    switch (type) {
        case json_type_null:
            return yaml_document_add_scalar(document, NULL, 
                                          (yaml_char_t *)"null", 
                                          4, 
                                          YAML_PLAIN_SCALAR_STYLE);

        case json_type_boolean: {
            const char *bool_str = json_object_get_boolean(json) ? "true" : "false";
            return yaml_document_add_scalar(document, NULL, 
                                          (yaml_char_t *)bool_str, 
                                          strlen(bool_str), 
                                          YAML_PLAIN_SCALAR_STYLE);
        }

        case json_type_int: {
            char buf[32];
            snprintf(buf, sizeof(buf), "%" PRId64, json_object_get_int64(json));
            return yaml_document_add_scalar(document, NULL, 
                                          (yaml_char_t *)buf, 
                                          strlen(buf), 
                                          YAML_PLAIN_SCALAR_STYLE);
        }

        case json_type_double: {
            char buf[64];
            double val = json_object_get_double(json);
            
            // Handle special case of -0.0
            if (val == 0.0 && signbit(val)) {
                strcpy(buf, "0.0");
            } else {
                // Use json-c's formatting but preserve .0 for whole numbers
                const char *json_str = json_object_to_json_string(json);
                strncpy(buf, json_str, sizeof(buf) - 1);
                buf[sizeof(buf) - 1] = '\0';
                
                // Check if this is a whole number that needs .0 suffix
                double int_part;
                if (modf(val, &int_part) == 0.0 && !strchr(buf, '.') && !strchr(buf, 'e') && !strchr(buf, 'E')) {
                    // It's a whole number without decimal point or exponent
                    size_t len = strlen(buf);
                    if (len < sizeof(buf) - 3) {
                        strcat(buf, ".0");
                    }
                }
            }
            
            return yaml_document_add_scalar(document, NULL, 
                                          (yaml_char_t *)buf, 
                                          strlen(buf), 
                                          YAML_PLAIN_SCALAR_STYLE);
        }

        case json_type_string: {
            const char *str = json_object_get_string(json);
            size_t len = json_object_get_string_len(json);
            
            // Determine if we need quotes
            yaml_scalar_style_t style = YAML_PLAIN_SCALAR_STYLE;
            
            // Check if string needs quoting
            if (len == 0 || strchr(str, '\n') || strchr(str, '\r') || 
                str[0] == ' ' || str[len-1] == ' ' ||
                strncmp(str, "null", len) == 0 ||
                strncmp(str, "true", len) == 0 ||
                strncmp(str, "false", len) == 0 ||
                strncmp(str, "yes", len) == 0 ||
                strncmp(str, "no", len) == 0 ||
                strncmp(str, "on", len) == 0 ||
                strncmp(str, "off", len) == 0) {
                style = YAML_DOUBLE_QUOTED_SCALAR_STYLE;
            }
            
            return yaml_document_add_scalar(document, NULL, 
                                          (yaml_char_t *)str, 
                                          len, 
                                          style);
        }

        case json_type_array:
            return yaml_add_array_to_document(document, json, error);

        case json_type_object:
            return yaml_add_object_to_document(document, json, error);

        default:
            buffer_sprintf(error, "Unknown JSON type: %d", type);
            return 0;
    }
}

bool json_to_yaml_document(struct json_object *json, yaml_document_t *document, BUFFER *error) {
    // Initialize with implicit_start=1 and implicit_end=1 to avoid "---" and "..."
    if (!yaml_document_initialize(document, NULL, NULL, NULL, 1, 1)) {
        buffer_strcat(error, "Failed to initialize YAML document");
        return false;
    }

    int root = yaml_add_json_to_document(document, json, error);
    if (!root) {
        yaml_document_delete(document);
        return false;
    }

    return true;
}

static bool yaml_generate_common(yaml_emitter_t *emitter, struct json_object *json, BUFFER *error) {
    yaml_document_t document;
    bool success = false;

    if (!json_to_yaml_document(json, &document, error)) {
        goto cleanup;
    }

    if (!yaml_emitter_open(emitter)) {
        buffer_strcat(error, "Failed to open YAML emitter");
        goto cleanup_doc;
    }

    if (!yaml_emitter_dump(emitter, &document)) {
        buffer_sprintf(error, "YAML emit error: %s", 
                      emitter->problem ? emitter->problem : "unknown error");
        goto cleanup_doc;
    }

    if (!yaml_emitter_close(emitter)) {
        buffer_strcat(error, "Failed to close YAML emitter");
        goto cleanup_doc;
    }

    success = true;

cleanup_doc:
    yaml_document_delete(&document);
cleanup:
    yaml_emitter_delete(emitter);
    return success;
}

static int yaml_write_handler_buffer(void *data, unsigned char *buffer, size_t size) {
    BUFFER *buf = (BUFFER *)data;
    buffer_memcat(buf, buffer, size);
    return 1;
}

bool yaml_generate_to_buffer(BUFFER *dst, struct json_object *json, BUFFER *error) {
    if (!dst) {
        buffer_strcat(error, "NULL destination buffer");
        return false;
    }

    yaml_emitter_t emitter;
    if (!yaml_emitter_initialize(&emitter)) {
        buffer_strcat(error, "Failed to initialize YAML emitter");
        return false;
    }

    yaml_emitter_set_output(&emitter, yaml_write_handler_buffer, dst);
    yaml_emitter_set_unicode(&emitter, 1);
    return yaml_generate_common(&emitter, json, error);
}

bool yaml_generate_to_filename(const char *filename, struct json_object *json, BUFFER *error) {
    if (!filename) {
        buffer_strcat(error, "NULL filename");
        return false;
    }

    FILE *file = fopen(filename, "w");
    if (!file) {
        buffer_sprintf(error, "Failed to open file '%s': %s", filename, strerror(errno));
        return false;
    }

    yaml_emitter_t emitter;
    if (!yaml_emitter_initialize(&emitter)) {
        buffer_strcat(error, "Failed to initialize YAML emitter");
        fclose(file);
        return false;
    }

    yaml_emitter_set_output_file(&emitter, file);
    yaml_emitter_set_unicode(&emitter, 1);
    bool result = yaml_generate_common(&emitter, json, error);
    fclose(file);
    return result;
}

bool yaml_generate_to_fd(int fd, struct json_object *json, BUFFER *error) {
    if (fd < 0) {
        buffer_strcat(error, "Invalid file descriptor");
        return false;
    }

    FILE *file = fdopen(dup(fd), "w");
    if (!file) {
        buffer_sprintf(error, "Failed to open file descriptor: %s", strerror(errno));
        return false;
    }

    yaml_emitter_t emitter;
    if (!yaml_emitter_initialize(&emitter)) {
        buffer_strcat(error, "Failed to initialize YAML emitter");
        fclose(file);
        return false;
    }

    yaml_emitter_set_output_file(&emitter, file);
    yaml_emitter_set_unicode(&emitter, 1);
    bool result = yaml_generate_common(&emitter, json, error);
    fclose(file);
    return result;
}