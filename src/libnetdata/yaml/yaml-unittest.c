// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "yaml.h"

static int test_yaml_parse_null(void) {
    int failed = 0;
    
    const char *yaml_inputs[] = {
        "null",
        "~",
        "---\nnull",
        NULL
    };
    
    for (int i = 0; yaml_inputs[i]; i++) {
        BUFFER *error = buffer_create(0, NULL);
        struct json_object *json = yaml_parse_string(yaml_inputs[i], error, YAML2JSON_DEFAULT);
        
        if (json != NULL) {
            fprintf(stderr, "FAILED: test_yaml_parse_null case %d: expected NULL, got %p\n", 
                    i, (void*)json);
            failed++;
            json_object_put(json);
        }
        
        buffer_free(error);
    }
    
    return failed;
}

static int test_yaml_parse_boolean(void) {
    int failed = 0;
    
    struct {
        const char *yaml;
        int expected;
    } test_cases[] = {
        {"true", 1},
        {"false", 0},
        {"yes", 1},
        {"no", 0},
        {"on", 1},
        {"off", 0},
        {"True", 1},
        {"False", 0},
        {"YES", 1},
        {"NO", 0},
        {NULL, 0}
    };
    
    for (int i = 0; test_cases[i].yaml; i++) {
        BUFFER *error = buffer_create(0, NULL);
        struct json_object *json = yaml_parse_string(test_cases[i].yaml, error, YAML2JSON_DEFAULT);
        
        if (!json || !json_object_is_type(json, json_type_boolean)) {
            fprintf(stderr, "FAILED: test_yaml_parse_boolean case %d: expected boolean for '%s'\n", 
                    i, test_cases[i].yaml);
            failed++;
        } else if (json_object_get_boolean(json) != test_cases[i].expected) {
            fprintf(stderr, "FAILED: test_yaml_parse_boolean case %d: expected %d, got %d for '%s'\n", 
                    i, test_cases[i].expected, json_object_get_boolean(json), test_cases[i].yaml);
            failed++;
        }
        
        if (json) json_object_put(json);
        buffer_free(error);
    }
    
    return failed;
}

static int test_yaml_parse_numbers(void) {
    int failed = 0;
    
    struct {
        const char *yaml;
        enum json_type expected_type;
        int64_t expected_int;
        double expected_double;
    } test_cases[] = {
        {"42", json_type_int, 42, 0},
        {"-123", json_type_int, -123, 0},
        {"0", json_type_int, 0, 0},
        {"3.14", json_type_double, 0, 3.14},
        {"-0.5", json_type_double, 0, -0.5},
        {"1.23e10", json_type_double, 0, 1.23e10},
        {"1.23e-10", json_type_double, 0, 1.23e-10},
        {NULL, 0, 0, 0}
    };
    
    for (int i = 0; test_cases[i].yaml; i++) {
        BUFFER *error = buffer_create(0, NULL);
        struct json_object *json = yaml_parse_string(test_cases[i].yaml, error, YAML2JSON_DEFAULT);
        
        if (!json || !json_object_is_type(json, test_cases[i].expected_type)) {
            fprintf(stderr, "FAILED: test_yaml_parse_numbers case %d: wrong type for '%s'\n", 
                    i, test_cases[i].yaml);
            failed++;
        } else {
            if (test_cases[i].expected_type == json_type_int) {
                if (json_object_get_int64(json) != test_cases[i].expected_int) {
                    fprintf(stderr, "FAILED: test_yaml_parse_numbers case %d: expected %" PRId64 ", got %" PRId64 " for '%s'\n", 
                            i, test_cases[i].expected_int, json_object_get_int64(json), test_cases[i].yaml);
                    failed++;
                }
            } else {
                double diff = fabs(json_object_get_double(json) - test_cases[i].expected_double);
                if (diff > 0.000001) {
                    fprintf(stderr, "FAILED: test_yaml_parse_numbers case %d: expected %f, got %f for '%s'\n", 
                            i, test_cases[i].expected_double, json_object_get_double(json), test_cases[i].yaml);
                    failed++;
                }
            }
        }
        
        if (json) json_object_put(json);
        buffer_free(error);
    }
    
    return failed;
}

static int test_yaml_parse_strings(void) {
    int failed = 0;
    
    struct {
        const char *yaml;
        const char *expected;
    } test_cases[] = {
        {"hello", "hello"},
        {"\"hello world\"", "hello world"},
        {"'hello world'", "hello world"},
        {"\"true\"", "true"},
        {"\"123\"", "123"},
        {"\"null\"", "null"},
        {"multi\\nline", "multi\\nline"},
        {"\"multi\\nline\"", "multi\nline"},
        {"\"  spaces  \"", "  spaces  "},
        {NULL, NULL}
    };
    
    for (int i = 0; test_cases[i].yaml; i++) {
        BUFFER *error = buffer_create(0, NULL);
        struct json_object *json = yaml_parse_string(test_cases[i].yaml, error, YAML2JSON_DEFAULT);
        
        if (!json || !json_object_is_type(json, json_type_string)) {
            fprintf(stderr, "FAILED: test_yaml_parse_strings case %d: expected string for '%s'\n", 
                    i, test_cases[i].yaml);
            failed++;
        } else if (strcmp(json_object_get_string(json), test_cases[i].expected) != 0) {
            fprintf(stderr, "FAILED: test_yaml_parse_strings case %d: expected '%s', got '%s' for '%s'\n", 
                    i, test_cases[i].expected, json_object_get_string(json), test_cases[i].yaml);
            failed++;
        }
        
        if (json) json_object_put(json);
        buffer_free(error);
    }
    
    return failed;
}

static int test_yaml_parse_arrays(void) {
    int failed = 0;
    
    // Try both flow and block style arrays
    const char *yaml = "- 1\n- 2\n- three\n- true\n- null\n- 4.5";
    
    BUFFER *error = buffer_create(0, NULL);
    struct json_object *json = yaml_parse_string(yaml, error, YAML2JSON_DEFAULT);
    
    if (!json) {
        fprintf(stderr, "FAILED: test_yaml_parse_arrays: json is NULL, error: %s\n", buffer_tostring(error));
        failed++;
        goto cleanup;
    }
    
    if (!json_object_is_type(json, json_type_array)) {
        fprintf(stderr, "FAILED: test_yaml_parse_arrays: expected array but got type %d\n", json_object_get_type(json));
        failed++;
        goto cleanup;
    }
    
    if (json_object_array_length(json) != 6) {
        fprintf(stderr, "FAILED: test_yaml_parse_arrays: expected 6 elements, got %zu\n", 
                json_object_array_length(json));
        failed++;
        goto cleanup;
    }
    
    // Check array elements
    struct json_object *elem;
    
    elem = json_object_array_get_idx(json, 0);
    if (!elem || !json_object_is_type(elem, json_type_int) || json_object_get_int64(elem) != 1) {
        fprintf(stderr, "FAILED: test_yaml_parse_arrays: element 0 check failed\n");
        failed++;
    }
    
    elem = json_object_array_get_idx(json, 2);
    if (!elem || !json_object_is_type(elem, json_type_string) || 
        strcmp(json_object_get_string(elem), "three") != 0) {
        fprintf(stderr, "FAILED: test_yaml_parse_arrays: element 2 check failed\n");
        failed++;
    }
    
    elem = json_object_array_get_idx(json, 3);
    if (!elem || !json_object_is_type(elem, json_type_boolean) || !json_object_get_boolean(elem)) {
        fprintf(stderr, "FAILED: test_yaml_parse_arrays: element 3 check failed\n");
        failed++;
    }
    
    elem = json_object_array_get_idx(json, 4);
    if (elem != NULL) {
        fprintf(stderr, "FAILED: test_yaml_parse_arrays: element 4 should be NULL (json-c represents null array elements as NULL)\n");
        failed++;
    }
    
cleanup:
    if (json) json_object_put(json);
    buffer_free(error);
    
    return failed;
}

static int test_yaml_parse_objects(void) {
    int failed = 0;
    
    const char *yaml = 
        "name: John Doe\n"
        "age: 30\n"
        "active: true\n"
        "salary: 50000.50\n"
        "address:\n"
        "  street: 123 Main St\n"
        "  city: Anytown\n"
        "tags:\n"
        "  - developer\n"
        "  - team-lead\n";
    
    BUFFER *error = buffer_create(0, NULL);
    struct json_object *json = yaml_parse_string(yaml, error, YAML2JSON_DEFAULT);
    
    if (!json || !json_object_is_type(json, json_type_object)) {
        fprintf(stderr, "FAILED: test_yaml_parse_objects: expected object\n");
        failed++;
        goto cleanup;
    }
    
    // Check object properties
    struct json_object *prop;
    
    if (!json_object_object_get_ex(json, "name", &prop) || 
        !json_object_is_type(prop, json_type_string) ||
        strcmp(json_object_get_string(prop), "John Doe") != 0) {
        fprintf(stderr, "FAILED: test_yaml_parse_objects: name property check failed\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(json, "age", &prop) || 
        !json_object_is_type(prop, json_type_int) ||
        json_object_get_int64(prop) != 30) {
        fprintf(stderr, "FAILED: test_yaml_parse_objects: age property check failed\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(json, "active", &prop) || 
        !json_object_is_type(prop, json_type_boolean) ||
        !json_object_get_boolean(prop)) {
        fprintf(stderr, "FAILED: test_yaml_parse_objects: active property check failed\n");
        failed++;
    }
    
    // Check nested object
    if (!json_object_object_get_ex(json, "address", &prop) || 
        !json_object_is_type(prop, json_type_object)) {
        fprintf(stderr, "FAILED: test_yaml_parse_objects: address property check failed\n");
        failed++;
    } else {
        struct json_object *street;
        if (!json_object_object_get_ex(prop, "street", &street) ||
            strcmp(json_object_get_string(street), "123 Main St") != 0) {
            fprintf(stderr, "FAILED: test_yaml_parse_objects: street property check failed\n");
            failed++;
        }
    }
    
    // Check array
    if (!json_object_object_get_ex(json, "tags", &prop) || 
        !json_object_is_type(prop, json_type_array) ||
        json_object_array_length(prop) != 2) {
        fprintf(stderr, "FAILED: test_yaml_parse_objects: tags property check failed\n");
        failed++;
    }
    
cleanup:
    if (json) json_object_put(json);
    buffer_free(error);
    
    return failed;
}

static int test_yaml_generation(void) {
    int failed = 0;
    
    // Create a more comprehensive JSON object
    struct json_object *root = json_object_new_object();
    json_object_object_add(root, "name", json_object_new_string("Test"));
    json_object_object_add(root, "version", json_object_new_int(1));
    json_object_object_add(root, "enabled", json_object_new_boolean(1));
    json_object_object_add(root, "pi", json_object_new_double(3.14159));
    // Test with NULL value
    json_object_object_add(root, "nothing", NULL);
    
    struct json_object *array = json_object_new_array();
    json_object_array_add(array, json_object_new_string("item1"));
    json_object_array_add(array, json_object_new_int(2));
    json_object_array_add(array, json_object_new_boolean(0));
    json_object_object_add(root, "items", array);
    
    struct json_object *nested = json_object_new_object();
    json_object_object_add(nested, "key", json_object_new_string("value"));
    json_object_object_add(root, "nested", nested);
    
    // Generate YAML
    BUFFER *output = buffer_create(0, NULL);
    BUFFER *error = buffer_create(0, NULL);
    
    if (!yaml_generate_to_buffer(output, root, error)) {
        fprintf(stderr, "FAILED: test_yaml_generation: failed to generate YAML: %s\n", 
                buffer_tostring(error));
        failed++;
        goto cleanup;
    }
    
    const char *yaml_str = buffer_tostring(output);
    if (!yaml_str || strlen(yaml_str) == 0) {
        fprintf(stderr, "FAILED: test_yaml_generation: generated empty YAML\n");
        failed++;
        goto cleanup;
    }
    
    // Parse the generated YAML back  
    buffer_flush(error);
    struct json_object *parsed = yaml_parse_string(yaml_str, error, YAML2JSON_DEFAULT);
    if (!parsed) {
        const char *err_msg = buffer_tostring(error);
        if (!err_msg || strlen(err_msg) == 0) {
            err_msg = "(no error message but result is NULL)";
        }
        fprintf(stderr, "FAILED: test_yaml_generation: failed to parse generated YAML: %s\nYAML was:\n%s\n", 
                err_msg, yaml_str);
        failed++;
        goto cleanup;
    }
    
    // Verify the parsed object matches the original
    struct json_object *prop;
    
    if (!json_object_object_get_ex(parsed, "name", &prop) ||
        strcmp(json_object_get_string(prop), "Test") != 0) {
        fprintf(stderr, "FAILED: test_yaml_generation: name property mismatch\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "version", &prop) ||
        json_object_get_int64(prop) != 1) {
        fprintf(stderr, "FAILED: test_yaml_generation: version property mismatch\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "enabled", &prop) ||
        !json_object_get_boolean(prop)) {
        fprintf(stderr, "FAILED: test_yaml_generation: enabled property mismatch\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "pi", &prop) ||
        !json_object_is_type(prop, json_type_double)) {
        fprintf(stderr, "FAILED: test_yaml_generation: pi property mismatch\n");
        failed++;
    }
    
    // Check NULL value - in json-c, JSON null is represented as C NULL
    if (!json_object_object_get_ex(parsed, "nothing", &prop)) {
        fprintf(stderr, "FAILED: test_yaml_generation: nothing property should exist\n");
        failed++;
    } else if (prop != NULL) {
        fprintf(stderr, "FAILED: test_yaml_generation: nothing property should be NULL\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "items", &prop) ||
        !json_object_is_type(prop, json_type_array) ||
        json_object_array_length(prop) != 3) {
        fprintf(stderr, "FAILED: test_yaml_generation: items property mismatch\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "nested", &prop) ||
        !json_object_is_type(prop, json_type_object)) {
        fprintf(stderr, "FAILED: test_yaml_generation: nested property mismatch\n");
        failed++;
    } else {
        struct json_object *nested_key;
        if (!json_object_object_get_ex(prop, "key", &nested_key) ||
            strcmp(json_object_get_string(nested_key), "value") != 0) {
            fprintf(stderr, "FAILED: test_yaml_generation: nested.key property mismatch\n");
            failed++;
        }
    }
    
    json_object_put(parsed);
    
cleanup:
    json_object_put(root);
    buffer_free(output);
    buffer_free(error);
    
    return failed;
}

static int test_yaml_parse_errors(void) {
    int failed = 0;
    
    const char *invalid_yaml[] = {
        "[unclosed array",
        "{ unclosed: object",
        NULL
    };
    
    for (int i = 0; invalid_yaml[i]; i++) {
        BUFFER *error = buffer_create(0, NULL);
        struct json_object *json = yaml_parse_string(invalid_yaml[i], error, YAML2JSON_DEFAULT);
        
        if (json != NULL || buffer_strlen(error) == 0) {
            fprintf(stderr, "FAILED: test_yaml_parse_errors case %d: expected parse error\n", i);
            failed++;
        }
        
        if (json) json_object_put(json);
        buffer_free(error);
    }
    
    return failed;
}

static int test_yaml_file_operations(void) {
    int failed = 0;
    
    const char *test_file = "/tmp/netdata_yaml_test.yaml";
    
    // Create a test JSON object
    struct json_object *root = json_object_new_object();
    json_object_object_add(root, "test", json_object_new_string("file operations"));
    json_object_object_add(root, "number", json_object_new_int(42));
    
    BUFFER *error = buffer_create(0, NULL);
    
    // Write to file
    if (!yaml_generate_to_filename(test_file, root, error)) {
        fprintf(stderr, "FAILED: test_yaml_file_operations: failed to write file: %s\n", 
                buffer_tostring(error));
        failed++;
        goto cleanup;
    }
    
    // Read from file
    struct json_object *parsed = yaml_parse_filename(test_file, error, YAML2JSON_DEFAULT);
    if (!parsed) {
        fprintf(stderr, "FAILED: test_yaml_file_operations: failed to read file: %s\n", 
                buffer_tostring(error));
        failed++;
        goto cleanup;
    }
    
    // Verify content
    struct json_object *prop;
    if (!json_object_object_get_ex(parsed, "test", &prop) ||
        strcmp(json_object_get_string(prop), "file operations") != 0) {
        fprintf(stderr, "FAILED: test_yaml_file_operations: test property mismatch\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "number", &prop) ||
        json_object_get_int64(prop) != 42) {
        fprintf(stderr, "FAILED: test_yaml_file_operations: number property mismatch\n");
        failed++;
    }
    
    json_object_put(parsed);
    
cleanup:
    // Cleanup
    unlink(test_file);
    json_object_put(root);
    buffer_free(error);
    
    return failed;
}

static int test_yaml_edge_cases(void) {
    int failed = 0;
    
    BUFFER *error = buffer_create(0, NULL);
    
    // Empty string
    struct json_object *json = yaml_parse_string("", error, YAML2JSON_DEFAULT);
    if (json != NULL) {
        fprintf(stderr, "FAILED: test_yaml_edge_cases: empty string should return NULL\n");
        failed++;
        json_object_put(json);
    }
    
    // NULL input
    buffer_flush(error);
    json = yaml_parse_string(NULL, error, YAML2JSON_DEFAULT);
    if (json != NULL || buffer_strlen(error) == 0) {
        fprintf(stderr, "FAILED: test_yaml_edge_cases: NULL input should fail\n");
        failed++;
        if (json) json_object_put(json);
    }
    
    // Empty object
    buffer_flush(error);
    json = yaml_parse_string("{}", error, YAML2JSON_DEFAULT);
    if (!json || !json_object_is_type(json, json_type_object) || 
        json_object_object_length(json) != 0) {
        fprintf(stderr, "FAILED: test_yaml_edge_cases: empty object parse failed\n");
        failed++;
    }
    if (json) json_object_put(json);
    
    // Empty array
    buffer_flush(error);
    json = yaml_parse_string("[]", error, YAML2JSON_DEFAULT);
    if (!json || !json_object_is_type(json, json_type_array) || 
        json_object_array_length(json) != 0) {
        fprintf(stderr, "FAILED: test_yaml_edge_cases: empty array parse failed\n");
        failed++;
    }
    if (json) json_object_put(json);
    
    buffer_free(error);
    
    return failed;
}

static int test_yaml_special_strings(void) {
    int failed = 0;
    
    // Create JSON with special strings
    struct json_object *root = json_object_new_object();
    json_object_object_add(root, "str_null", json_object_new_string("null"));
    json_object_object_add(root, "str_true", json_object_new_string("true"));
    json_object_object_add(root, "str_false", json_object_new_string("false"));
    json_object_object_add(root, "str_yes", json_object_new_string("yes"));
    json_object_object_add(root, "str_no", json_object_new_string("no"));
    json_object_object_add(root, "str_on", json_object_new_string("on"));
    json_object_object_add(root, "str_off", json_object_new_string("off"));
    json_object_object_add(root, "str_spaces", json_object_new_string("  spaces  "));
    json_object_object_add(root, "str_newline", json_object_new_string("line1\nline2"));
    json_object_object_add(root, "str_empty", json_object_new_string(""));
    
    BUFFER *output = buffer_create(0, NULL);
    BUFFER *error = buffer_create(0, NULL);
    
    // Generate YAML
    if (!yaml_generate_to_buffer(output, root, error)) {
        fprintf(stderr, "FAILED: test_yaml_special_strings: failed to generate YAML: %s\n", 
                buffer_tostring(error));
        failed++;
        goto cleanup;
    }
    
    // Parse back
    struct json_object *parsed = yaml_parse_string(buffer_tostring(output), error, YAML2JSON_DEFAULT);
    if (!parsed) {
        fprintf(stderr, "FAILED: test_yaml_special_strings: failed to parse YAML: %s\n", 
                buffer_tostring(error));
        failed++;
        goto cleanup;
    }
    
    // Verify all strings are preserved correctly
    struct json_object *prop;
    
    if (!json_object_object_get_ex(parsed, "str_null", &prop) ||
        !json_object_is_type(prop, json_type_string) ||
        strcmp(json_object_get_string(prop), "null") != 0) {
        fprintf(stderr, "FAILED: test_yaml_special_strings: str_null mismatch\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "str_true", &prop) ||
        !json_object_is_type(prop, json_type_string) ||
        strcmp(json_object_get_string(prop), "true") != 0) {
        fprintf(stderr, "FAILED: test_yaml_special_strings: str_true mismatch\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "str_spaces", &prop) ||
        !json_object_is_type(prop, json_type_string) ||
        strcmp(json_object_get_string(prop), "  spaces  ") != 0) {
        fprintf(stderr, "FAILED: test_yaml_special_strings: str_spaces mismatch\n");
        failed++;
    }
    
    if (!json_object_object_get_ex(parsed, "str_empty", &prop) ||
        !json_object_is_type(prop, json_type_string) ||
        strcmp(json_object_get_string(prop), "") != 0) {
        fprintf(stderr, "FAILED: test_yaml_special_strings: str_empty mismatch\n");
        failed++;
    }
    
    json_object_put(parsed);
    
cleanup:
    json_object_put(root);
    buffer_free(output);
    buffer_free(error);
    
    return failed;
}

// Forward declaration
int yaml_comprehensive_unittest(void);

int yaml_unittest(void) {
    int passed = 0;
    int failed = 0;
    
    printf("Starting YAML parser/generator unit tests\n");
    printf("=========================================\n\n");
    
    // Run all tests
    struct {
        const char *name;
        int (*test_func)(void);
    } tests[] = {
        {"test_yaml_parse_null", test_yaml_parse_null},
        {"test_yaml_parse_boolean", test_yaml_parse_boolean},
        {"test_yaml_parse_numbers", test_yaml_parse_numbers},
        {"test_yaml_parse_strings", test_yaml_parse_strings},
        {"test_yaml_parse_arrays", test_yaml_parse_arrays},
        {"test_yaml_parse_objects", test_yaml_parse_objects},
        {"test_yaml_generation", test_yaml_generation},
        {"test_yaml_parse_errors", test_yaml_parse_errors},
        {"test_yaml_file_operations", test_yaml_file_operations},
        {"test_yaml_edge_cases", test_yaml_edge_cases},
        {"test_yaml_special_strings", test_yaml_special_strings},
        {NULL, NULL}
    };
    
    for (int i = 0; tests[i].name; i++) {
        printf("Running %s...\n", tests[i].name);
        int test_failed = tests[i].test_func();
        if (test_failed == 0) {
            printf("  PASSED\n");
            passed++;
        } else {
            printf("  FAILED (%d failures)\n", test_failed);
            failed += test_failed;
        }
    }
    
    printf("\n=========================================\n");
    printf("YAML unit tests summary:\n");
    printf("  Tests run: %d\n", passed + (failed > 0 ? 1 : 0));
    printf("  Passed: %d\n", passed);
    printf("  Failed: %d\n", failed);
    printf("=========================================\n");
    
    // Run comprehensive tests
    int comprehensive_failed = yaml_comprehensive_unittest();
    failed += comprehensive_failed;
    
    return failed;
}