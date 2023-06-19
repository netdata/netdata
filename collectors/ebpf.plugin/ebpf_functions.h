// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_FUNCTIONS_H
#define NETDATA_EBPF_FUNCTIONS_H 1

// configuration file & description
#define NETDATA_DIRECTORY_FUNCTIONS_CONFIG_FILE "functions.conf"
#define NETDATA_EBPF_FUNCTIONS_MODULE_DESC "Show information about current function status."

// function list
#define EBPF_FUNCTION_THREAD "ebpf_thread"

#define EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION "Detailed information about eBPF threads."
#define EBPF_PLUGIN_THREAD_FUNCTION_ERROR_THREAD_NOT_FOUND "ebpf.plugin does not have thread named "

#define EBPF_PLUGIN_FUNCTIONS(NAME, DESC) do { \
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " \"" NAME "\" 10 \"%s\"\n", DESC); \
} while(0)

#define EBPF_THREADS_SELECT_THREAD "thread:"
#define EBPF_THREADS_ENABLE_CATEGORY "enable:"
#define EBPF_THREADS_DISABLE_CATEGORY "disable:"

#define EBPF_THREAD_STATUS_RUNNING "running"
#define EBPF_THREAD_STATUS_STOPPED "stopped"

// function list
#define EBPF_FUNCTION_THREAD "ebpf_thread"

#define EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION "Detailed information on the currently running threads."

#define EBPF_PLUGIN_FUNCTIONS(NAME, DESC) do { \
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " \"" NAME "\" 10 \"%s\"\n", DESC); \
} while(0)

// This function should be unified with add_table_field in apps.plugin
#define ebpf_add_table_field(wb, key, name, visible, type, visualization, transform, decimal_points, units, max, sort, sortable, sticky, unique_key, pointer_to, summary, range) do { \
    buffer_json_member_add_object(wb, key);                                                                     \
    buffer_json_member_add_uint64(wb, "index", fields_added);                                                   \
    buffer_json_member_add_boolean(wb, "unique_key", unique_key);                                               \
    buffer_json_member_add_string(wb, "name", name);                                                            \
    buffer_json_member_add_boolean(wb, "visible", visible);                                                     \
    buffer_json_member_add_string(wb, "type", type);                                                            \
    buffer_json_member_add_string_or_omit(wb, "units", (char*)(units));                                         \
    buffer_json_member_add_string(wb, "visualization", visualization);                                          \
    buffer_json_member_add_object(wb, "value_options");                                                         \
    buffer_json_member_add_string_or_omit(wb, "units", (char*)(units));                                         \
    buffer_json_member_add_string(wb, "transform", transform);                                                  \
    buffer_json_member_add_uint64(wb, "decimal_points", decimal_points);                                        \
    buffer_json_object_close(wb);                                                                               \
    if(!isnan((NETDATA_DOUBLE)(max)))                                                                           \
        buffer_json_member_add_double(wb, "max", (NETDATA_DOUBLE)(max));                                        \
    buffer_json_member_add_string_or_omit(wb, "pointer_to", (char *)(pointer_to));                              \
    buffer_json_member_add_string(wb, "sort", sort);                                                            \
    buffer_json_member_add_boolean(wb, "sortable", sortable);                                                   \
    buffer_json_member_add_boolean(wb, "sticky", sticky);                                                       \
    buffer_json_member_add_string(wb, "summary", summary);                                                      \
    buffer_json_member_add_string(wb, "filter", (range)?"range":"multiselect");                                 \
    buffer_json_object_close(wb);                                                                               \
    fields_added++;                                                                                             \
} while(0)

#define EBPF_THREADS_ENABLE_CATEGORY "enable:"

void *ebpf_function_thread(void *ptr);

#endif
