// SPDX-License-Identifier: GPL-3.0-or-later

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"
#include "collectors/plugins.d/local-sockets.h"

#include "network-viewer-userspace-bpf.h"
#include "network-viewer.skel.h"

#include <bpf/libbpf.h>
#include "bpf/bpf.h"

#define NETWORK_VIEWER_HELP "Network dependencies (outbound connections)"

static void bpf_log(const char *format, ...) __attribute__ ((format(__printf__, 1, 2)));
static void bpf_log(const char *format, ...) {
    char buf[16384];
    va_list args;
    va_start(args, format);
    vsnprintf(buf, sizeof(buf), format, args);
    va_end(args);

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "NETWORK-VIEWER: %s", buf);
}

netdata_mutex_t stdout_mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

struct network_viewer_globals {
    struct network_viewer_bpf *skel;
    int connections_map_fd;
} network_viewer_globals = { 0 };

void network_viewer_function(const char *transaction, char *function __maybe_unused, usec_t *stop_monotonic_ut __maybe_unused,
                             bool *cancelled __maybe_unused, BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
                             const char *source __maybe_unused, void *data __maybe_unused) {

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_string(wb, "help", NETWORK_VIEWER_HELP);
    buffer_json_member_add_array(wb, "data");
    {
        int map_fd = network_viewer_globals.connections_map_fd;

        char key[bpf_connection_key_size()];
        char next_key[bpf_connection_key_size()];
        char value[bpf_connection_data_size()];

        memset(key, 0, sizeof(key));
        memset(next_key, 0, sizeof(next_key));
        memset(value, 0, sizeof(value));

        for( ; bpf_map_get_next_key(map_fd, key, next_key) == 0; memcpy(key, next_key, sizeof(key))) {
            if (bpf_map_lookup_elem(map_fd, next_key, value) == 0) {
                BPF_CONNECTION c;
                populate_connection_from_key_and_data(&c, next_key, value);

                if(c.protocol != IPPROTO_TCP && c.protocol != IPPROTO_UDP)
                    continue;

                char local_address[INET6_ADDRSTRLEN];
                char remote_address[INET6_ADDRSTRLEN];
                char *type;

                if(c.family == AF_INET) {
                    ipv4_address_to_txt(c.local.ip.ipv4, local_address);
                    ipv4_address_to_txt(c.remote.ip.ipv4, remote_address);
                    type = c.protocol == IPPROTO_TCP ? "tcp4" : "udp4";
                }
                else if(c.family == AF_INET6) {
                    ipv6_address_to_txt(&c.local.ip.ipv6, local_address);
                    ipv6_address_to_txt(&c.remote.ip.ipv6, remote_address);
                    type = c.protocol == IPPROTO_TCP ? "tcp6" : "udp6";
                }
                else
                    continue;

                buffer_json_add_array_item_array(wb);
                {
                    buffer_json_add_array_item_string(wb, type);
                    buffer_json_add_array_item_uint64(wb, c.pid);
                    buffer_json_add_array_item_string(wb, c.comm);
                    buffer_json_add_array_item_string(wb, local_address);
                    buffer_json_add_array_item_uint64(wb, c.local.port);
                    buffer_json_add_array_item_string(wb, remote_address);
                    buffer_json_add_array_item_uint64(wb, c.remote.port);
                    buffer_json_add_array_item_uint64(wb, c.first_seen_s * MSEC_PER_SEC);
                    buffer_json_add_array_item_uint64(wb, c.last_seen_s * MSEC_PER_SEC);
                    buffer_json_add_array_item_string(wb, c.type == 0 ? "pre-existing" : "accurate");
                }
                buffer_json_array_close(wb);

            } else
                bpf_log("failed to get key from the kernel");
        }
    }
    buffer_json_array_close(wb);
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        // Type
        buffer_rrdf_table_add_field(wb, field_id++, "Type", "Socket Type",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // Pid
        buffer_rrdf_table_add_field(wb, field_id++, "PID", "Process ID",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // Comm
        buffer_rrdf_table_add_field(wb, field_id++, "Comm", "Command Name",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // Local Address
        buffer_rrdf_table_add_field(wb, field_id++, "L.IP", "Local IP Address",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // Local Port
        buffer_rrdf_table_add_field(wb, field_id++, "L.Port", "Local Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // Remote Address
        buffer_rrdf_table_add_field(wb, field_id++, "Remote IP", "Remote IP Address",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // Remote Port
        buffer_rrdf_table_add_field(wb, field_id++, "R.Port", "Remote Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // First Seen
        buffer_rrdf_table_add_field(wb, field_id++, "F.Seen", "First Seen",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // Last Seen
        buffer_rrdf_table_add_field(wb, field_id++, "L.Seen", "Last Seen",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

        // LifeTime
        buffer_rrdf_table_add_field(wb, field_id++, "Life", "Socket Life Time",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE,
                                    NULL);

    }
    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "L.Seen");

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", now_realtime_sec(), wb);
    netdata_mutex_unlock(&stdout_mutex);
}

// ----------------------------------------------------------------------------------------------------------------
// initialize ebpf hashtable with existing outbound connections

static void send_to_ebpf_hashtable(LS_STATE *ls __maybe_unused, LOCAL_SOCKET *n, void *data) {
    time_t *now = data;

    BPF_CONNECTION c = {
        .protocol = n->protocol,
        .family = n->family,
        .pid = n->pid,
        .state = n->state,
        .type = 0,
        .first_seen_s = *now,
        .last_seen_s = *now,
        .local = {
            .port = n->local.port
        },
        .remote = {
            .port = n->remote.port
        },
    };

    if(n->family == AF_INET) {
        c.local.ip.ipv4 = n->local.ip.ipv4;
        c.remote.ip.ipv4 = n->remote.ip.ipv4;
    }
    else if(n->family == AF_INET6) {
        c.local.ip.ipv6 = n->local.ip.ipv6;
        c.remote.ip.ipv6 = n->remote.ip.ipv6;
    }

    strncpyz(c.comm, n->comm, sizeof(c.comm) - 1);

    char k[bpf_connection_key_size()];
    char d[bpf_connection_data_size()];
    populate_connection_key_and_data(k, d, &c);

    int ret = bpf_map_update_elem(network_viewer_globals.connections_map_fd, k, d, BPF_ANY);
    if(ret != 0)
        bpf_log("cannot add entry into hash map.");
}

// ----------------------------------------------------------------------------------------------------------------
// ebpf setup and cleanup

void ebpf_cleanup(int code) {
    network_viewer_bpf__detach(network_viewer_globals.skel);
    network_viewer_bpf__destroy(network_viewer_globals.skel);
    exit(code);
}

static inline struct bpf_link *attach_kprobe(struct bpf_object *obj, const char *function) {
    struct bpf_program *prog = bpf_object__find_program_by_name(obj, function);
    if (!prog) {
        bpf_log("failed to find '%s' ebpf program", function);
        return NULL;
    }

    struct bpf_link *link = bpf_program__attach(prog);
    if (libbpf_get_error(link)) {
        bpf_log("failed to attach '%s' kprobe", function);
        return NULL;
    }

    return link;
}

void initialize_ebpf(void) {
    network_viewer_globals.skel = network_viewer_bpf__open_and_load();
    if (!network_viewer_globals.skel) {
        bpf_log("failed to open and load BPF skeleton");
        exit(1);
    }

    if (network_viewer_bpf__attach(network_viewer_globals.skel)) {
        bpf_log("failed to attach BPF skeleton");
        network_viewer_bpf__destroy(network_viewer_globals.skel);
        exit(1);
    }

    // Get the file descriptor of the map
    network_viewer_globals.connections_map_fd = bpf_map__fd(network_viewer_globals.skel->maps.connections);
    if (network_viewer_globals.connections_map_fd < 0) {
        bpf_log("failed to get the connection map fd");
        goto cleanup;
    }

    return;

cleanup:
    ebpf_cleanup(1);
    exit(1);
}

// ----------------------------------------------------------------------------------------------------------------
// signal handler

static void setup_signal_handler(void) {
    struct sigaction sa;
    sa.sa_handler = ebpf_cleanup;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;

    sigaction(SIGINT, &sa, NULL);
    sigaction(SIGTERM, &sa, NULL);
}

// ----------------------------------------------------------------------------------------------------------------
// main

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    clocks_init();
    netdata_thread_set_tag("NETWORK-VIEWER");
    nd_log_initialize_for_external_plugins("network-viewer.plugin");

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix(true) == -1) exit(1);

    // ----------------------------------------------------------------------------------------------------------------

    setup_signal_handler();
    initialize_ebpf();

    // ----------------------------------------------------------------------------------------------------------------

    time_t now = now_realtime_sec();

    LS_STATE ls = {
        .config = {
            .listening = false,
            .inbound = false,
            .outbound = true,
            .local = false,
            .tcp4 = true,
            .tcp6 = true,
            .udp4 = true,
            .udp6 = true,
            .pid = true,
            .cmdline = false,
            .comm = true,

            .max_errors = 10,

            .cb = send_to_ebpf_hashtable,
            .data = &now,
        },
        .stats = { 0 },
        .sockets_hashtable = { 0 },
        .local_ips_hashtable = { 0 },
        .listening_ports_hashtable = { 0 },
    };

    local_sockets_process(&ls);

    // ----------------------------------------------------------------------------------------------------------------

    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n",
            "network-dependencies", 10, NETWORK_VIEWER_HELP,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);

    // ----------------------------------------------------------------------------------------------------------------

    struct functions_evloop_globals *wg =
        functions_evloop_init(1, "Network-Viewer", &stdout_mutex, &plugin_should_exit);

    functions_evloop_add_function(wg,
                                  "network-dependencies",
                                  network_viewer_function,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT,
                                  NULL);

    // ----------------------------------------------------------------------------------------------------------------

    usec_t step_ut = 100 * USEC_PER_MS;
    usec_t send_newline_ut = 0;
    bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!plugin_should_exit) {

        usec_t dt_ut = heartbeat_next(&hb, step_ut);
        send_newline_ut += dt_ut;

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    ebpf_cleanup(0);
    return 0;
}
