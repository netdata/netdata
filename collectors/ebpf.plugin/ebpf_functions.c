// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_functions.h"

/*****************************************************************
 *  EBPF SELECT MODULE
 *****************************************************************/

/**
 * Select Module
 *
 * @param thread_name name of the thread we are looking for.
 *
 * @return it returns a pointer for the module that has thread_name on success or NULL otherwise.
 */
ebpf_module_t *ebpf_functions_select_module(const char *thread_name) {
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        if (strcmp(ebpf_modules[i].thread_name, thread_name) == 0) {
            return &ebpf_modules[i];
        }
    }

    return NULL;
}

/*****************************************************************
 *  EBPF HELP FUNCTIONS
 *****************************************************************/

/**
 * Thread Help
 *
 * Shows help with all options accepted by thread function.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
*/
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
            "Process thread is not controlled by functions until we finish the creation of functions per thread..\n"
            );

    pthread_mutex_lock(&lock);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
    pthread_mutex_unlock(&lock);

    buffer_free(wb);
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
static void ebpf_function_error(const char *transaction, int code, const char *msg) {
    pthread_mutex_lock(&lock);
    pluginsd_function_json_error_to_stdout(transaction, code, msg);
    pthread_mutex_unlock(&lock);
}

/*****************************************************************
 *  EBPF THREAD FUNCTION
 *****************************************************************/

/**
 * Function enable
 *
 * Enable a specific thread.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
 * @param function     function name and arguments given to thread.
 * @param line_buffer  buffer used to parse args
 * @param line_max     Number of arguments given
 * @param timeout      The function timeout
 * @param em           The structure with thread information
 */
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
                struct netdata_static_thread *st = lem->thread;
                // Load configuration again
                ebpf_update_module(lem, default_btf, running_on_kernel, isrh);

                // another request for thread that already ran, cleanup and restart
                if (st->thread)
                    freez(st->thread);

                if (period <= 0)
                    period = EBPF_DEFAULT_LIFETIME;

                st->thread = mallocz(sizeof(netdata_thread_t));
                lem->enabled = NETDATA_THREAD_EBPF_FUNCTION_RUNNING;
                lem->lifetime = period;

#ifdef NETDATA_INTERNAL_CHECKS
                netdata_log_info("Starting thread %s with lifetime = %d", thread_name, period);
#endif

                netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT,
                                      st->start_routine, lem);
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
        buffer_json_add_array_item_string(wb, wem->thread_name);

        // description
        buffer_json_add_array_item_string(wb, wem->thread_description);
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
                             RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);

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
    pthread_mutex_lock(&lock);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", expires, wb);
    pthread_mutex_unlock(&lock);

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
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    char buffer[PLUGINSD_LINE_MAX + 1];

    char *s = NULL;
    while(!ebpf_exit_plugin && (s = fgets(buffer, PLUGINSD_LINE_MAX, stdin))) {
        char *words[PLUGINSD_MAX_WORDS] = { NULL };
        size_t num_words = quoted_strings_splitter_pluginsd(buffer, words, PLUGINSD_MAX_WORDS);

        const char *keyword = get_word(words, num_words, 0);

        if(keyword && strcmp(keyword, PLUGINSD_KEYWORD_FUNCTION) == 0) {
            char *transaction = get_word(words, num_words, 1);
            char *timeout_s = get_word(words, num_words, 2);
            char *function = get_word(words, num_words, 3);

            if(!transaction || !*transaction || !timeout_s || !*timeout_s || !function || !*function) {
                netdata_log_error("Received incomplete %s (transaction = '%s', timeout = '%s', function = '%s'). Ignoring it.",
                                  keyword,
                                  transaction?transaction:"(unset)",
                                  timeout_s?timeout_s:"(unset)",
                                  function?function:"(unset)");
            }
            else {
                int timeout = str2i(timeout_s);
                if (!strncmp(function, EBPF_FUNCTION_THREAD, sizeof(EBPF_FUNCTION_THREAD) - 1))
                    ebpf_function_thread_manipulation(transaction,
                                                      function,
                                                      buffer,
                                                      PLUGINSD_LINE_MAX + 1,
                                                      timeout,
                                                      em);
                else
                    ebpf_function_error(transaction,
                                        HTTP_RESP_NOT_FOUND,
                                        "No function with this name found in ebpf.plugin.");
            }
        }
        else
            netdata_log_error("Received unknown command: %s", keyword ? keyword : "(unset)");
    }
    return NULL;
}
