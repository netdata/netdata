// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_functions.h"

/*****************************************************************
 *  EBPF FUNCTION COMMON
 *****************************************************************/

/**
 * Function Start thread
 *
 * Start a specific thread after user request.
 *
 * @param em           The structure with thread information
 * @param period
 * @return
 */
static int ebpf_function_start_thread(ebpf_module_t *em, int period)
{
    struct netdata_static_thread *st = em->thread;
    // another request for thread that already ran, cleanup and restart
    if (period <= 0)
        period = EBPF_DEFAULT_LIFETIME;

    st->thread = NULL;
    em->enabled = NETDATA_THREAD_EBPF_FUNCTION_RUNNING;
    em->lifetime = period;

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info("Starting thread %s with lifetime = %d", em->info.thread_name, period);
#endif

    st->thread = nd_thread_create(st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, em);
    return st->thread ? 0 : 1;
}

/*****************************************************************
 *  EBPF ERROR FUNCTIONS
 *****************************************************************/

/**
 * Function error
 *
 * Show error when a wrong function is given
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
 * @param code         the error code to show with the message.
 * @param msg          the error message
 */
static inline void ebpf_function_error(const char *transaction, int code, const char *msg)
{
    pluginsd_function_json_error_to_stdout(transaction, code, msg);
}

/**
 * Thread Help
 *
 * Shows help with all options accepted by thread function.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
*/
static inline void ebpf_function_help(const char *transaction, const char *message)
{
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600);
    fprintf(stdout, "%s", message);
    pluginsd_function_result_end_to_stdout();
    fflush(stdout);
}

/*****************************************************************
 *  EBPF SOCKET FUNCTION
 *****************************************************************/

/**
 * Fill Fake socket
 *
 * Fill socket with an invalid request.
 *
 * @param fake_values is the structure where we are storing the value.
 */
static inline void ebpf_socket_fill_fake_socket(netdata_socket_plus_t *fake_values)
{
    snprintfz(fake_values->socket_string.src_ip, INET6_ADDRSTRLEN, "%s", "127.0.0.1");
    snprintfz(fake_values->socket_string.dst_ip, INET6_ADDRSTRLEN, "%s", "127.0.0.1");
    fake_values->pid = getpid();
    //fake_values->socket_string.src_port = 0;
    fake_values->socket_string.dst_port[0] = 0;
    snprintfz(fake_values->socket_string.dst_ip, NI_MAXSERV, "%s", "none");
    fake_values->data.family = AF_INET;
    fake_values->data.protocol = AF_UNSPEC;
}

static NETDATA_DOUBLE bytes_to_mb(uint64_t bytes)
{
    return (NETDATA_DOUBLE)bytes / (1024 * 1024);
}

/**
 * Fill function buffer
 *
 * Fill buffer with data to be shown on cloud.
 *
 * @param wb          buffer where we store data.
 * @param values      data read from hash table
 * @param name        the process name
 */
static void ebpf_fill_function_buffer(BUFFER *wb, netdata_socket_plus_t *values, char *name)
{
    buffer_json_add_array_item_array(wb);

    // IMPORTANT!
    // THE ORDER SHOULD BE THE SAME WITH THE FIELDS!

    // PID
    buffer_json_add_array_item_uint64(wb, (uint64_t)values->pid);

    // NAME
    if (!values->data.name[0])
        buffer_json_add_array_item_string(wb, (name) ? name : "unknown");
    else
        buffer_json_add_array_item_string(wb, values->data.name);

    // Origin
    buffer_json_add_array_item_string(wb, (values->data.external_origin) ? "in" : "out");

    // Source IP
    buffer_json_add_array_item_string(wb, values->socket_string.src_ip);

    // SRC Port
    //buffer_json_add_array_item_uint64(wb, (uint64_t) values->socket_string.src_port);

    // Destination IP
    buffer_json_add_array_item_string(wb, values->socket_string.dst_ip);

    // DST Port
    buffer_json_add_array_item_string(wb, values->socket_string.dst_port);

    uint64_t connections;
    if (values->data.protocol == IPPROTO_TCP) {
        buffer_json_add_array_item_string(wb, "TCP");
        buffer_json_add_array_item_double(wb, bytes_to_mb(values->data.tcp.tcp_bytes_received));
        buffer_json_add_array_item_double(wb, bytes_to_mb(values->data.tcp.tcp_bytes_sent));
        connections = values->data.tcp.ipv4_connect + values->data.tcp.ipv6_connect;
    } else if (values->data.protocol == IPPROTO_UDP) {
        buffer_json_add_array_item_string(wb, "UDP");
        buffer_json_add_array_item_double(wb, bytes_to_mb(values->data.udp.udp_bytes_received));
        buffer_json_add_array_item_double(wb, bytes_to_mb(values->data.udp.udp_bytes_sent));
        connections = values->data.udp.call_udp_sent + values->data.udp.call_udp_received;
    } else {
        buffer_json_add_array_item_string(wb, "UNSPEC");
        buffer_json_add_array_item_double(wb, 0);
        buffer_json_add_array_item_double(wb, 0);
        connections = 1;
    }

    // Connections
    if (values->flags & NETDATA_SOCKET_FLAGS_ALREADY_OPEN) {
        connections++;
    } else if (!connections) {
        // If no connections, this means that we lost when connection was opened
        values->flags |= NETDATA_SOCKET_FLAGS_ALREADY_OPEN;
        connections++;
    }
    buffer_json_add_array_item_uint64(wb, connections);

    buffer_json_array_close(wb);
}

/**
 * Clean Judy array unsafe
 *
 * Clean all Judy Array allocated to show table when a function is called.
 * Before to call this function it is necessary to lock `ebpf_judy_pid.index.rw_spinlock`.
 **/
static void ebpf_socket_clean_judy_array_unsafe()
{
    if (!ebpf_judy_pid.index.JudyLArray)
        return;

    Pvoid_t *pid_value, *socket_value;
    Word_t local_pid = 0, local_socket = 0;
    bool first_pid = true, first_socket = true;
    while ((pid_value = JudyLFirstThenNext(ebpf_judy_pid.index.JudyLArray, &local_pid, &first_pid))) {
        netdata_ebpf_judy_pid_stats_t *pid_ptr = (netdata_ebpf_judy_pid_stats_t *)*pid_value;
        rw_spinlock_write_lock(&pid_ptr->socket_stats.rw_spinlock);
        if (pid_ptr->socket_stats.JudyLArray) {
            while (
                (socket_value = JudyLFirstThenNext(pid_ptr->socket_stats.JudyLArray, &local_socket, &first_socket))) {
                netdata_socket_plus_t *socket_clean = *socket_value;
                aral_freez(aral_socket_table, socket_clean);
            }
            JudyLFreeArray(&pid_ptr->socket_stats.JudyLArray, PJE0);
            pid_ptr->socket_stats.JudyLArray = NULL;
        }
        rw_spinlock_write_unlock(&pid_ptr->socket_stats.rw_spinlock);
    }
}

/**
 * Fill function buffer unsafe
 *
 * Fill the function buffer with socket information. Before to call this function it is necessary to lock
 * ebpf_judy_pid.index.rw_spinlock
 *
 * @param buf    buffer used to store data to be shown by function.
 *
 * @return it returns 0 on success and -1 otherwise.
 */
static void ebpf_socket_fill_function_buffer_unsafe(BUFFER *buf)
{
    int counter = 0;

    Pvoid_t *pid_value, *socket_value;
    Word_t local_pid = 0;
    bool first_pid = true;
    while ((pid_value = JudyLFirstThenNext(ebpf_judy_pid.index.JudyLArray, &local_pid, &first_pid))) {
        netdata_ebpf_judy_pid_stats_t *pid_ptr = (netdata_ebpf_judy_pid_stats_t *)*pid_value;
        bool first_socket = true;
        Word_t local_timestamp = 0;
        rw_spinlock_read_lock(&pid_ptr->socket_stats.rw_spinlock);
        if (pid_ptr->socket_stats.JudyLArray) {
            while ((
                socket_value = JudyLFirstThenNext(pid_ptr->socket_stats.JudyLArray, &local_timestamp, &first_socket))) {
                netdata_socket_plus_t *values = (netdata_socket_plus_t *)*socket_value;
                ebpf_fill_function_buffer(buf, values, pid_ptr->cmdline);
            }
            counter++;
        }
        rw_spinlock_read_unlock(&pid_ptr->socket_stats.rw_spinlock);
    }

    if (!counter) {
        netdata_socket_plus_t fake_values = {};
        ebpf_socket_fill_fake_socket(&fake_values);
        ebpf_fill_function_buffer(buf, &fake_values, NULL);
    }
}

/**
 * Socket read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data on very busy socket.
 *
 * @param buf the buffer to store data;
 * @param em  the module main structure.
 *
 * @return It always returns NULL.
 */
void ebpf_socket_read_open_connections(BUFFER *buf, struct ebpf_module *em)
{
    // thread was not initialized or Array was reset
    rw_spinlock_read_lock(&ebpf_judy_pid.index.rw_spinlock);
    if (!em->maps || (em->maps[NETDATA_SOCKET_OPEN_SOCKET].map_fd == ND_EBPF_MAP_FD_NOT_INITIALIZED) ||
        !ebpf_judy_pid.index.JudyLArray) {
        netdata_socket_plus_t fake_values = {};

        ebpf_socket_fill_fake_socket(&fake_values);

        ebpf_fill_function_buffer(buf, &fake_values, NULL);
        rw_spinlock_read_unlock(&ebpf_judy_pid.index.rw_spinlock);
        return;
    }

    rw_spinlock_read_lock(&network_viewer_opt.rw_spinlock);
    ebpf_socket_fill_function_buffer_unsafe(buf);
    rw_spinlock_read_unlock(&network_viewer_opt.rw_spinlock);
    rw_spinlock_read_unlock(&ebpf_judy_pid.index.rw_spinlock);
}

/**
 * Function: Socket
 *
 * Show information for sockets stored in hash tables.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
 * @param function     function name and arguments given to thread.
 * @param timeout      The function timeout
 * @param cancelled    Variable used to store function status.
 */
static void ebpf_function_socket_manipulation(
    const char *transaction,
    char *function __maybe_unused,
    usec_t *stop_monotonic_ut __maybe_unused,
    bool *cancelled __maybe_unused,
    BUFFER *payload __maybe_unused,
    HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused,
    void *data __maybe_unused)
{
    ebpf_module_t *em = &ebpf_modules[EBPF_MODULE_SOCKET_IDX];

    char *words[PLUGINSD_MAX_WORDS] = {NULL};
    size_t num_words = quoted_strings_splitter_whitespace(function, words, PLUGINSD_MAX_WORDS);
    const char *name;
    int period = -1;
    rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
    network_viewer_opt.enabled = CONFIG_BOOLEAN_YES;
    uint32_t previous;
    bool info = false;
    time_t now_s = now_realtime_sec();

    static const char *socket_help = {
        "ebpf.plugin / socket\n"
        "\n"
        "Function `socket` display information for all open sockets during ebpf.plugin runtime.\n"
        "During thread runtime the plugin is always collecting data, but when an option is modified, the plugin\n"
        "resets completely the previous table and can show a clean data for the first request before to bring the\n"
        "modified request.\n"
        "\n"
        "The following filters are supported:\n"
        "\n"
        "   family:FAMILY\n"
        "      Shows information for the FAMILY specified. Option accepts IPV4, IPV6 and all, that is the default.\n"
        "\n"
        "   period:PERIOD\n"
        "      Enable socket to run a specific PERIOD in seconds. When PERIOD is not\n"
        "      specified plugin will use the default 300 seconds\n"
        "\n"
        "   resolve:BOOL\n"
        "      Resolve service name, default value is YES.\n"
        "\n"
        "   range:CIDR\n"
        "      Show sockets that have only a specific destination. Default all addresses.\n"
        "\n"
        "   port:range\n"
        "      Show sockets that have only a specific destination.\n"
        "\n"
        "   reset\n"
        "      Send a reset to collector. When a collector receives this command, it uses everything defined in configuration file.\n"
        "\n"
        "   interfaces\n"
        "      When the collector receives this command, it read all available interfaces on host.\n"
        "\n"
        "Filters can be combined. Each filter can be given only one time. Default all ports\n"};

    for (int i = 1; i < PLUGINSD_MAX_WORDS; i++) {
        const char *keyword = get_word(words, num_words, i);
        if (!keyword)
            break;

        if (strncmp(keyword, EBPF_FUNCTION_SOCKET_FAMILY, sizeof(EBPF_FUNCTION_SOCKET_FAMILY) - 1) == 0) {
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_FAMILY) - 1];
            previous = network_viewer_opt.family;
            uint32_t family = AF_UNSPEC;
            if (!strcmp(name, "IPV4"))
                family = AF_INET;
            else if (!strcmp(name, "IPV6"))
                family = AF_INET6;

            if (family != previous) {
                rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
                network_viewer_opt.family = family;
                rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);
                ebpf_socket_clean_judy_array_unsafe();
            }
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_PERIOD, sizeof(EBPF_FUNCTION_SOCKET_PERIOD) - 1) == 0) {
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_PERIOD) - 1];
            pthread_mutex_lock(&ebpf_exit_cleanup);
            period = str2i(name);
            if (period > 0) {
                em->lifetime = period;
            } else
                em->lifetime = EBPF_NON_FUNCTION_LIFE_TIME;

#ifdef NETDATA_DEV_MODE
            collector_info("Lifetime modified for %u", em->lifetime);
#endif
            pthread_mutex_unlock(&ebpf_exit_cleanup);
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_RESOLVE, sizeof(EBPF_FUNCTION_SOCKET_RESOLVE) - 1) == 0) {
            previous = network_viewer_opt.service_resolution_enabled;
            uint32_t resolution;
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_RESOLVE) - 1];
            resolution = (!strcasecmp(name, "YES")) ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;

            if (previous != resolution) {
                rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
                network_viewer_opt.service_resolution_enabled = resolution;
                rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);

                ebpf_socket_clean_judy_array_unsafe();
            }
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_RANGE, sizeof(EBPF_FUNCTION_SOCKET_RANGE) - 1) == 0) {
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_RANGE) - 1];
            rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
            ebpf_clean_ip_structure(&network_viewer_opt.included_ips);
            ebpf_clean_ip_structure(&network_viewer_opt.excluded_ips);
            ebpf_parse_ips_unsafe((char *)name);
            rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);

            ebpf_socket_clean_judy_array_unsafe();
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_PORT, sizeof(EBPF_FUNCTION_SOCKET_PORT) - 1) == 0) {
            name = &keyword[sizeof(EBPF_FUNCTION_SOCKET_PORT) - 1];
            rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
            ebpf_clean_port_structure(&network_viewer_opt.included_port);
            ebpf_clean_port_structure(&network_viewer_opt.excluded_port);
            ebpf_parse_ports((char *)name);
            rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);

            ebpf_socket_clean_judy_array_unsafe();
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_RESET, sizeof(EBPF_FUNCTION_SOCKET_RESET) - 1) == 0) {
            rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
            ebpf_clean_port_structure(&network_viewer_opt.included_port);
            ebpf_clean_port_structure(&network_viewer_opt.excluded_port);

            ebpf_clean_ip_structure(&network_viewer_opt.included_ips);
            ebpf_clean_ip_structure(&network_viewer_opt.excluded_ips);
            ebpf_clean_ip_structure(&network_viewer_opt.ipv4_local_ip);
            ebpf_clean_ip_structure(&network_viewer_opt.ipv6_local_ip);

            parse_network_viewer_section(&socket_config);
            ebpf_read_local_addresses_unsafe();
            network_viewer_opt.enabled = CONFIG_BOOLEAN_YES;
            rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);
        } else if (strncmp(keyword, EBPF_FUNCTION_SOCKET_INTERFACES, sizeof(EBPF_FUNCTION_SOCKET_INTERFACES) - 1) == 0) {
            rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
            ebpf_read_local_addresses_unsafe();
            rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);
        } else if (strncmp(keyword, "help", 4) == 0) {
            ebpf_function_help(transaction, socket_help);
            rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);
            return;
        } else if (strncmp(keyword, "info", 4) == 0)
            info = true;
    }
    rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

    if (em->enabled > NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        // Cleanup when we already had a thread running
        rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
        ebpf_socket_clean_judy_array_unsafe();
        rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

        collect_pids |= 1 << EBPF_MODULE_SOCKET_IDX;
        pthread_mutex_lock(&ebpf_exit_cleanup);
        if (ebpf_function_start_thread(em, period)) {
            ebpf_function_error(transaction, HTTP_RESP_INTERNAL_SERVER_ERROR, "Cannot start thread.");
            pthread_mutex_unlock(&ebpf_exit_cleanup);
            return;
        }
    } else {
        pthread_mutex_lock(&ebpf_exit_cleanup);
        if (period < 0)
            em->lifetime = (em->enabled != NETDATA_THREAD_EBPF_FUNCTION_RUNNING) ? EBPF_NON_FUNCTION_LIFE_TIME :
                                                                                   EBPF_DEFAULT_LIFETIME;
    }
    pthread_mutex_unlock(&ebpf_exit_cleanup);

    BUFFER *wb = buffer_create(4096, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", em->update_every);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", EBPF_PLUGIN_SOCKET_FUNCTION_DESCRIPTION);

    if (info)
        goto close_and_send;

    // Collect data
    buffer_json_member_add_array(wb, "data");
    ebpf_socket_read_open_connections(wb, em);
    buffer_json_array_close(wb); // data

    buffer_json_member_add_object(wb, "columns");
    {
        int fields_id = 0;

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE VALUES!
        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "PID",
            "Process ID",
            RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NUMBER,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Name",
            "Process Name",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_FULL_WIDTH,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Origin",
            "Connection Origin",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Src",
            "Source IP Address",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        /*
        buffer_rrdf_table_add_field(wb, fields_id++, "SrcPort", "Source Port", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
                                    NULL);
                                    */

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Dst",
            "Destination IP Address",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "DstPort",
            "Destination Port",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Protocol",
            "Transport Layer Protocol",
            RRDF_FIELD_TYPE_STRING,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NONE,
            0,
            NULL,
            NAN,
            RRDF_FIELD_SORT_ASCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_NONE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Rcvd",
            "Traffic Received",
            RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NUMBER,
            3,
            "MB",
            NAN,
            RRDF_FIELD_SORT_DESCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_NONE,
            RRDF_FIELD_OPTS_VISIBLE,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id++,
            "Sent",
            "Traffic Sent",
            RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NUMBER,
            3,
            "MB",
            NAN,
            RRDF_FIELD_SORT_DESCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_NONE,
            RRDF_FIELD_OPTS_VISIBLE,
            NULL);

        buffer_rrdf_table_add_field(
            wb,
            fields_id,
            "Conns",
            "Connections",
            RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE,
            RRDF_FIELD_TRANSFORM_NUMBER,
            0,
            "connections",
            NAN,
            RRDF_FIELD_SORT_DESCENDING,
            NULL,
            RRDF_FIELD_SUMMARY_SUM,
            RRDF_FIELD_FILTER_NONE,
            RRDF_FIELD_OPTS_VISIBLE,
            NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Rcvd");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Traffic");
        {
            buffer_json_member_add_string(wb, "name", "Traffic");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Rcvd");
                buffer_json_add_array_item_string(wb, "Sent");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Connections");
        {
            buffer_json_member_add_string(wb, "name", "Connections");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Conns");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Traffic");
        buffer_json_add_array_item_string(wb, "Name");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Connections");
        buffer_json_add_array_item_string(wb, "Name");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "Name");
        {
            buffer_json_member_add_string(wb, "name", "Process Name");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Name");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Origin");
        {
            buffer_json_member_add_string(wb, "name", "Origin");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Origin");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Src");
        {
            buffer_json_member_add_string(wb, "name", "Source IP");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Src");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Dst");
        {
            buffer_json_member_add_string(wb, "name", "Destination IP");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Dst");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "DstPort");
        {
            buffer_json_member_add_string(wb, "name", "Destination Port");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "DstPort");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Protocol");
        {
            buffer_json_member_add_string(wb, "name", "Protocol");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Protocol");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

close_and_send:
    buffer_json_member_add_time_t(wb, "expires", now_s + em->update_every);
    buffer_json_finalize(wb);

    // Lock necessary to avoid race condition
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "application/json", now_s + em->update_every);

    fwrite(buffer_tostring(wb), buffer_strlen(wb), 1, stdout);

    pluginsd_function_result_end_to_stdout();
    fflush(stdout);

    buffer_free(wb);
}

/*****************************************************************
 *  EBPF FUNCTION THREAD
 *****************************************************************/

/**
 * FUNCTION thread.
 *
 * @param ptr a `ebpf_module_t *`.
 *
 * @return always NULL.
 */
void ebpf_function_thread(void *ptr)
{
    (void)ptr;

    struct functions_evloop_globals *wg = functions_evloop_init(1, "EBPF", &lock, &ebpf_plugin_exit);

    functions_evloop_add_function(
        wg, EBPF_FUNCTION_SOCKET, ebpf_function_socket_manipulation, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);

    pthread_mutex_lock(&lock);
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *em = &ebpf_modules[i];
        if (!em->functions.fnct_routine)
            continue;

        EBPF_PLUGIN_FUNCTIONS(em->functions.fcnt_name, em->functions.fcnt_desc, em->update_every);
    }
    pthread_mutex_unlock(&lock);

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop()) {
        heartbeat_next(&hb);

        if (ebpf_plugin_stop()) {
            break;
        }
    }
}
