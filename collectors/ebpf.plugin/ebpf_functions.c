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
    if (st->thread)
        freez(st->thread);

    if (period <= 0)
        period = EBPF_DEFAULT_LIFETIME;

    st->thread = mallocz(sizeof(netdata_thread_t));
    em->enabled = NETDATA_THREAD_EBPF_FUNCTION_RUNNING;
    em->lifetime = period;

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info("Starting thread %s with lifetime = %d", em->info.thread_name, period);
#endif

    return netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, em);
}

/*****************************************************************
 *  EBPF SELECT MODULE
 *****************************************************************/

/**
 * Select Module
 *
 * @param thread_name name of the thread we are looking for.
 *
 * @return it returns a pointer for the module that has thread_name on success or NULL otherwise.
ebpf_module_t *ebpf_functions_select_module(const char *thread_name) {
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        if (strcmp(ebpf_modules[i].info.thread_name, thread_name) == 0) {
            return &ebpf_modules[i];
        }
    }

    return NULL;
}
 */

/*****************************************************************
 *  EBPF HELP FUNCTIONS
 *****************************************************************/

/**
 * Thread Help
 *
 * Shows help with all options accepted by thread function.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
static void ebpf_function_thread_manipulation_help(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb, "%s",
            "ebpf.plugin / thread\n"
            "\n"
            "Function `thread` allows user to control eBPF threads.\n"
            "\n"
            "The following filters are supported:\n"
            "\n"
            "   thread:NAME\n"
            "      Shows information for the thread NAME. Names are listed inside `ebpf.d.conf`.\n"
            "\n"
            "   enable:NAME:PERIOD\n"
            "      Enable a specific thread named `NAME` to run a specific PERIOD in seconds. When PERIOD is not\n"
            "      specified plugin will use the default 300 seconds\n"
            "\n"
            "   disable:NAME\n"
            "      Disable a sp.\n"
            "\n"
            "Filters can be combined. Each filter can be given only one time.\n"
            );

    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);

    buffer_free(wb);
}
*/

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
static void ebpf_function_error(const char *transaction, int code, const char *msg) {
    pluginsd_function_json_error_to_stdout(transaction, code, msg);
}

/*****************************************************************
 *  EBPF THREAD FUNCTION
 *****************************************************************/

/**
 * Function: thread
 *
 * Enable a specific thread.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
 * @param function     function name and arguments given to thread.
 * @param line_buffer  buffer used to parse args
 * @param line_max     Number of arguments given
 * @param timeout      The function timeout
 * @param em           The structure with thread information
static void ebpf_function_thread_manipulation(const char *transaction,
                                              char *function __maybe_unused,
                                              char *line_buffer __maybe_unused,
                                              int line_max __maybe_unused,
                                              int timeout __maybe_unused,
                                              ebpf_module_t *em)
{
    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    char message[512];
    uint32_t show_specific_thread = 0;
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, PLUGINSD_MAX_WORDS);
    for(int i = 1; i < PLUGINSD_MAX_WORDS ;i++) {
        const char *keyword = get_word(words, num_words, i);
        if (!keyword)
            break;

        ebpf_module_t *lem;
        if(strncmp(keyword, EBPF_THREADS_ENABLE_CATEGORY, sizeof(EBPF_THREADS_ENABLE_CATEGORY) -1) == 0) {
            char thread_name[128];
            int period = -1;
            const char *name = &keyword[sizeof(EBPF_THREADS_ENABLE_CATEGORY) - 1];
            char *separator = strchr(name, ':');
            if (separator) {
                strncpyz(thread_name, name, separator - name);
                period = str2i(++separator);
            } else {
                strncpyz(thread_name, name, strlen(name));
            }

            lem = ebpf_functions_select_module(thread_name);
            if (!lem) {
                snprintfz(message, 511, "%s%s", EBPF_PLUGIN_THREAD_FUNCTION_ERROR_THREAD_NOT_FOUND, name);
                ebpf_function_error(transaction, HTTP_RESP_NOT_FOUND, message);
                return;
            }

            pthread_mutex_lock(&ebpf_exit_cleanup);
            if (lem->enabled > NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
                // Load configuration again
                ebpf_update_module(lem, default_btf, running_on_kernel, isrh);

                if (ebpf_function_start_thread(lem, period)) {
                    ebpf_function_error(transaction,
                                        HTTP_RESP_INTERNAL_SERVER_ERROR,
                                        "Cannot start thread.");
                    return;
                }
            } else {
                lem->running_time = 0;
                if (period > 0) // user is modifying period to run
                    lem->lifetime = period;
#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("Thread %s had lifetime updated for %d", thread_name, period);
#endif
            }
            pthread_mutex_unlock(&ebpf_exit_cleanup);
        } else if(strncmp(keyword, EBPF_THREADS_DISABLE_CATEGORY, sizeof(EBPF_THREADS_DISABLE_CATEGORY) -1) == 0) {
            const char *name = &keyword[sizeof(EBPF_THREADS_DISABLE_CATEGORY) - 1];
            lem = ebpf_functions_select_module(name);
            if (!lem) {
                snprintfz(message, 511, "%s%s", EBPF_PLUGIN_THREAD_FUNCTION_ERROR_THREAD_NOT_FOUND, name);
                ebpf_function_error(transaction, HTTP_RESP_NOT_FOUND, message);
                return;
            }

            pthread_mutex_lock(&ebpf_exit_cleanup);
            if (lem->enabled < NETDATA_THREAD_EBPF_STOPPING && lem->thread->thread) {
                lem->lifetime = 0;
                lem->running_time = lem->update_every;
                netdata_thread_cancel(*lem->thread->thread);
            }
            pthread_mutex_unlock(&ebpf_exit_cleanup);
        } else if(strncmp(keyword, EBPF_THREADS_SELECT_THREAD, sizeof(EBPF_THREADS_SELECT_THREAD) -1) == 0) {
            const char *name = &keyword[sizeof(EBPF_THREADS_SELECT_THREAD) - 1];
            lem = ebpf_functions_select_module(name);
            if (!lem) {
                snprintfz(message, 511, "%s%s", EBPF_PLUGIN_THREAD_FUNCTION_ERROR_THREAD_NOT_FOUND, name);
                ebpf_function_error(transaction, HTTP_RESP_NOT_FOUND, message);
                return;
            }

            show_specific_thread |= 1<<lem->thread_id;
        } else if(strncmp(keyword, "help", 4) == 0) {
            ebpf_function_thread_manipulation_help(transaction);
            return;
        }
    }

    time_t expires = now_realtime_sec() + em->update_every;

    BUFFER *wb = buffer_create(PLUGINSD_LINE_MAX, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", em->update_every);
    buffer_json_member_add_string(wb, "help", EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION);

    // Collect data
    buffer_json_member_add_array(wb, "data");
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        if (show_specific_thread && !(show_specific_thread & 1<<i))
            continue;

        ebpf_module_t *wem = &ebpf_modules[i];
        buffer_json_add_array_item_array(wb);

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE FIELDS!

        // thread name
        buffer_json_add_array_item_string(wb, wem->info.thread_name);

        // description
        buffer_json_add_array_item_string(wb, wem->info.thread_description);
        // Either it is not running or received a disabled signal and it is stopping.
        if (wem->enabled > NETDATA_THREAD_EBPF_FUNCTION_RUNNING ||
            (!wem->lifetime && (int)wem->running_time == wem->update_every)) {
            // status
            buffer_json_add_array_item_string(wb, EBPF_THREAD_STATUS_STOPPED);

            // Time remaining
            buffer_json_add_array_item_uint64(wb, 0);

            // action
            buffer_json_add_array_item_string(wb, "NULL");
        } else {
            // status
            buffer_json_add_array_item_string(wb, EBPF_THREAD_STATUS_RUNNING);

            // Time remaining
            buffer_json_add_array_item_uint64(wb, (wem->lifetime) ? (wem->lifetime - wem->running_time) : 0);

            // action
            buffer_json_add_array_item_string(wb, "Enabled/Disabled");
        }

        buffer_json_array_close(wb);
    }

    buffer_json_array_close(wb); // data

    buffer_json_member_add_object(wb, "columns");
    {
        int fields_id = 0;

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE VALUES!
        buffer_rrdf_table_add_field(wb, fields_id++, "Thread", "Thread Name", RRDF_FIELD_TYPE_STRING,
                             RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                             RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                             RRDF_FIELD_FILTER_MULTISELECT,
                             RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_UNIQUE_KEY, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Description", "Thread Desc", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Status", "Thread Status", RRDF_FIELD_TYPE_STRING,
                             RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                             RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                             RRDF_FIELD_FILTER_MULTISELECT,
                             RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Time", "Time Remaining", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL,
                                    NAN, RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Action", "Thread Action", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Thread");

    buffer_json_member_add_object(wb, "charts");
    {
        // Threads
        buffer_json_member_add_object(wb, "eBPFThreads");
        {
            buffer_json_member_add_string(wb, "name", "Threads");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Threads");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Life Time
        buffer_json_member_add_object(wb, "eBPFLifeTime");
        {
            buffer_json_member_add_string(wb, "name", "LifeTime");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Threads");
                buffer_json_add_array_item_string(wb, "Time");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    // Do we use only on fields that can be groupped?
    buffer_json_member_add_object(wb, "group_by");
    {
        // group by Status
        buffer_json_member_add_object(wb, "Status");
        {
            buffer_json_member_add_string(wb, "name", "Thread status");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Status");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", expires);
    buffer_json_finalize(wb);

    // Lock necessary to avoid race condition
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", expires, wb);

    buffer_free(wb);
}
 */

/*****************************************************************
 *  EBPF SOCKET FUNCTION
 *****************************************************************/

/**
 * Thread Help
 *
 * Shows help with all options accepted by thread function.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
*/
static void ebpf_function_socket_help(const char *transaction) {
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600);
    fprintf(stdout, "%s",
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
            "Filters can be combined. Each filter can be given only one time. Default all ports\n"
    );
    pluginsd_function_result_end_to_stdout();
    fflush(stdout);
}

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
    buffer_json_add_array_item_string(wb, (name) ? name : "not identified");

    // Origin
    buffer_json_add_array_item_string(wb, (values->data.external_origin) ? "incoming" : "outgoing");

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
        // Protocol
        buffer_json_add_array_item_string(wb, "TCP");

        // Bytes received
        buffer_json_add_array_item_uint64(wb, (uint64_t) values->data.tcp.tcp_bytes_received);

        // Bytes sent
        buffer_json_add_array_item_uint64(wb, (uint64_t) values->data.tcp.tcp_bytes_sent);

        // Connections
        connections = values->data.tcp.ipv4_connect + values->data.tcp.ipv6_connect;
    } else if (values->data.protocol == IPPROTO_UDP) {
        // Protocol
        buffer_json_add_array_item_string(wb, "UDP");

        // Bytes received
        buffer_json_add_array_item_uint64(wb, (uint64_t) values->data.udp.udp_bytes_received);

        // Bytes sent
        buffer_json_add_array_item_uint64(wb, (uint64_t) values->data.udp.udp_bytes_sent);

        // Connections
        connections = values->data.udp.call_udp_sent + values->data.udp.call_udp_received;
    } else {
        // Protocol
        buffer_json_add_array_item_string(wb, "UNSPEC");

        // Bytes received
        buffer_json_add_array_item_uint64(wb, 0);

        // Bytes sent
        buffer_json_add_array_item_uint64(wb, 0);

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
            while ((socket_value = JudyLFirstThenNext(pid_ptr->socket_stats.JudyLArray, &local_socket, &first_socket))) {
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
            while ((socket_value = JudyLFirstThenNext(pid_ptr->socket_stats.JudyLArray, &local_timestamp, &first_socket))) {
                netdata_socket_plus_t *values = (netdata_socket_plus_t *)*socket_value;
                ebpf_fill_function_buffer(buf, values, pid_ptr->cmdline);
            }
            counter++;
        }
        rw_spinlock_read_unlock(&pid_ptr->socket_stats.rw_spinlock);
    }

    if (!counter) {
        netdata_socket_plus_t fake_values = { };
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
        !ebpf_judy_pid.index.JudyLArray){
        netdata_socket_plus_t fake_values = { };

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
static void ebpf_function_socket_manipulation(const char *transaction,
                                              char *function __maybe_unused,
                                              int timeout __maybe_unused,
                                              bool *cancelled __maybe_unused)
{
    UNUSED(timeout);
    ebpf_module_t *em = &ebpf_modules[EBPF_MODULE_SOCKET_IDX];

    char *words[PLUGINSD_MAX_WORDS] = {NULL};
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, PLUGINSD_MAX_WORDS);
    const char *name;
    int period = -1;
    rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
    network_viewer_opt.enabled = CONFIG_BOOLEAN_YES;
    uint32_t previous;

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
            ebpf_function_socket_help(transaction);
            rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);
            return;
        }
    }
    rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

    pthread_mutex_lock(&ebpf_exit_cleanup);
    if (em->enabled > NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        // Cleanup when we already had a thread running
        rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
        ebpf_socket_clean_judy_array_unsafe();
        rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

        if (ebpf_function_start_thread(em, period)) {
            ebpf_function_error(transaction,
                                HTTP_RESP_INTERNAL_SERVER_ERROR,
                                "Cannot start thread.");
            pthread_mutex_unlock(&ebpf_exit_cleanup);
            return;
        }
    } else {
        if (period < 0 && em->lifetime < EBPF_NON_FUNCTION_LIFE_TIME) {
            em->lifetime = EBPF_NON_FUNCTION_LIFE_TIME;
        }
    }
    pthread_mutex_unlock(&ebpf_exit_cleanup);

    time_t expires = now_realtime_sec() + em->update_every;

    BUFFER *wb = buffer_create(PLUGINSD_LINE_MAX, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, false);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", em->update_every);
    buffer_json_member_add_string(wb, "help", EBPF_PLUGIN_SOCKET_FUNCTION_DESCRIPTION);

    // Collect data
    buffer_json_member_add_array(wb, "data");
    ebpf_socket_read_open_connections(wb, em);
    buffer_json_array_close(wb); // data

    buffer_json_member_add_object(wb, "columns");
    {
        int fields_id = 0;

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE VALUES!
        buffer_rrdf_table_add_field(wb, fields_id++, "PID", "Process ID", RRDF_FIELD_TYPE_INTEGER,
            RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
            RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
            RRDF_FIELD_FILTER_MULTISELECT,
            RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
            NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Process Name", "Process Name", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Origin", "The connection origin.", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Request from", "Request from IP", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        /*
        buffer_rrdf_table_add_field(wb, fields_id++, "SRC PORT", "Source Port", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
                                    NULL);
                                    */

        buffer_rrdf_table_add_field(wb, fields_id++, "Destination IP", "Destination IP", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Destination Port", "Destination Port", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Protocol", "Communication protocol", RRDF_FIELD_TYPE_STRING,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Incoming Bandwidth", "Bytes received.", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
                                    NULL);

        buffer_rrdf_table_add_field(wb, fields_id++, "Outgoing Bandwidth", "Bytes sent.", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
                                    NULL);

        buffer_rrdf_table_add_field(wb, fields_id, "Connections", "Number of calls to tcp_vX_connections and udp_sendmsg, where X is the protocol version.", RRDF_FIELD_TYPE_INTEGER,
                                    RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER, 0, NULL, NAN,
                                    RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                                    RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY,
                                    NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_object(wb, "charts");
    {
        // OutBound Connections
        buffer_json_member_add_object(wb, "IPInboundConn");
        {
            buffer_json_member_add_string(wb, "name", "TCP Inbound Connection");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "connected_tcp");
                buffer_json_add_array_item_string(wb, "connected_udp");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // OutBound Connections
        buffer_json_member_add_object(wb, "IPTCPOutboundConn");
        {
            buffer_json_member_add_string(wb, "name", "TCP Outbound Connection");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "connected_V4");
                buffer_json_add_array_item_string(wb, "connected_V6");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // TCP Functions
        buffer_json_member_add_object(wb, "TCPFunctions");
        {
            buffer_json_member_add_string(wb, "name", "TCPFunctions");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "received");
                buffer_json_add_array_item_string(wb, "sent");
                buffer_json_add_array_item_string(wb, "close");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // TCP Bandwidth
        buffer_json_member_add_object(wb, "TCPBandwidth");
        {
            buffer_json_member_add_string(wb, "name", "TCPBandwidth");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "received");
                buffer_json_add_array_item_string(wb, "sent");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // UDP Functions
        buffer_json_member_add_object(wb, "UDPFunctions");
        {
            buffer_json_member_add_string(wb, "name", "UDPFunctions");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "received");
                buffer_json_add_array_item_string(wb, "sent");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // UDP Bandwidth
        buffer_json_member_add_object(wb, "UDPBandwidth");
        {
            buffer_json_member_add_string(wb, "name", "UDPBandwidth");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "received");
                buffer_json_add_array_item_string(wb, "sent");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_string(wb, "default_sort_column", "PID");

    // Do we use only on fields that can be groupped?
    buffer_json_member_add_object(wb, "group_by");
    {
        // group by PID
        buffer_json_member_add_object(wb, "PID");
        {
            buffer_json_member_add_string(wb, "name", "Process ID");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "PID");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by Process Name
        buffer_json_member_add_object(wb, "Process Name");
        {
            buffer_json_member_add_string(wb, "name", "Process Name");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Process Name");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by Process Name
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

        // group by Request From IP
        buffer_json_member_add_object(wb, "Request from");
        {
            buffer_json_member_add_string(wb, "name", "Request from IP");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Request from");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by Destination IP
        buffer_json_member_add_object(wb, "Destination IP");
        {
            buffer_json_member_add_string(wb, "name", "Destination IP");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Destination IP");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by DST Port
        buffer_json_member_add_object(wb, "Destination Port");
        {
            buffer_json_member_add_string(wb, "name", "Destination Port");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Destination Port");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // group by Protocol
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

    buffer_json_member_add_time_t(wb, "expires", expires);
    buffer_json_finalize(wb);

    // Lock necessary to avoid race condition
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "application/json", expires);

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
void *ebpf_function_thread(void *ptr)
{
    (void)ptr;

    bool ebpf_function_plugin_exit = false;
    struct functions_evloop_globals *wg = functions_evloop_init(1,
                                                                "EBPF",
                                                                &lock,
                                                                &ebpf_function_plugin_exit);

    functions_evloop_add_function(wg,
                                  "ebpf_socket",
                                  ebpf_function_socket_manipulation,
                                  PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT);

    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!ebpf_exit_plugin) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);

        if (ebpf_function_plugin_exit) {
            pthread_mutex_lock(&ebpf_exit_cleanup);
            ebpf_stop_threads(0);
            pthread_mutex_unlock(&ebpf_exit_cleanup);
            break;
        }
    }

    return NULL;
}
