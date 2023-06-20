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
    for (i = 0; ebpf_modules[i].thread_name; i++) {
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
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600);
    fprintf(stdout, "%s",
            "ebpf.plugin / thread\n"
            "\n"
            "Function `thread` allows user to control eBPF threads.\n"
            "\n"
            "The following filters are supported:\n"
            "\n"
            "   thread:NAME\n"
            "      Shows information for the thread NAME. Names are listed inside `ebpf.d.conf`.\n"
            "\n"
            "   enable:NAME\n"
            "      Enable a specific thread to run a specific period of time `NAME`.\n"
            "\n"
            "   disable:NAME\n"
            "      Disable a sp.\n"
            "\n"
            "Filters can be combined. Each filter can be given only one time.\n"
            );
    pluginsd_function_result_end_to_stdout();
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
    char buffer[PLUGINSD_LINE_MAX + 1];
    json_escape_string(buffer, msg, PLUGINSD_LINE_MAX);

    pluginsd_function_result_begin_to_stdout(transaction, code, "application/json", now_realtime_sec());
    fprintf(stdout, "{\"status\":%d,\"error_message\":\"%s\"}", code, buffer);
    pluginsd_function_result_end_to_stdout();
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
    size_t num_words = pluginsd_split_words(function, words, PLUGINSD_MAX_WORDS);
    for(int i = 1; i < PLUGINSD_MAX_WORDS ;i++) {
        const char *keyword = get_word(words, num_words, i);
        if (!keyword)
            break;

        if(strncmp(keyword, EBPF_THREADS_ENABLE_CATEGORY, sizeof(EBPF_THREADS_ENABLE_CATEGORY) -1) == 0) {
            const char *name = &keyword[sizeof(EBPF_THREADS_ENABLE_CATEGORY) - 1];
            ebpf_module_t *em = ebpf_functions_select_module(name);
            if (!em) {
                snprintfz(message, 511, "%s%s", "ebpf.plugin does not have thread with name ", name);
                ebpf_function_error(transaction, HTTP_RESP_NOT_FOUND, message);
                return;
            }

            pthread_mutex_lock(&ebpf_exit_cleanup);
            if (em->enabled != NETDATA_THREAD_EBPF_RUNNING && !em->thread->thread) {
                struct netdata_static_thread *st = em->thread;
                // Load configuration again
                ebpf_update_module(em, default_btf, running_on_kernel, isrh);

                st->thread = mallocz(sizeof(netdata_thread_t));
                em->thread_id = i;
                em->enabled = NETDATA_THREAD_EBPF_RUNNING;

                netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, em);

                if (em->apps_charts && em->apps_routine && em->maps && apps_groups_root_target) {
                    /**
                     * TODO: APPS CREATION NEEDS MORE CHANGES IN THE CODE, SO I AM POSTPONING FOR NEXT PR
                     */
                }
            } else
                em->running_time = 0;
            pthread_mutex_unlock(&ebpf_exit_cleanup);
        } else if(strncmp(keyword, EBPF_THREADS_DISABLE_CATEGORY, sizeof(EBPF_THREADS_DISABLE_CATEGORY) -1) == 0) {
            /**
             * TODO: TO DISABLE PROPERLY A THREAD WE MUST OBSOLETE CHARTS, SO I WILL BRING IN ANOTHER PR
             */
        } else if(strncmp(keyword, "help", 4) == 0) {
            ebpf_function_thread_manipulation_help(transaction);
            return;
        }
    }

    time_t expires = now_realtime_sec() + em->update_every;
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "application/json", expires);

    BUFFER *wb = buffer_create(PLUGINSD_LINE_MAX, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, false);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", em->update_every);
    buffer_json_member_add_string(wb, "help", EBPF_PLUGIN_THREAD_FUNCTION_DESCRIPTION);

    // Collect data
    buffer_json_member_add_array(wb, "data");
    pthread_mutex_lock(&ebpf_exit_cleanup);
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_module_t *wem = &ebpf_modules[i];
        buffer_json_add_array_item_array(wb);

        // IMPORTANT!
        // THE ORDER SHOULD BE THE SAME WITH THE FIELDS!

        // thread name
        buffer_json_add_array_item_string(wb, wem->thread_name);

        // enabled
        buffer_json_add_array_item_string(wb,
                                          (wem->enabled != NETDATA_THREAD_EBPF_NOT_RUNNING) ?
                                          "running":
                                          "stopped");

        buffer_json_array_close(wb);
    }
    pthread_mutex_unlock(&ebpf_exit_cleanup);

    buffer_json_array_close(wb);

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
        buffer_rrdf_table_add_field(wb, fields_id++, "Status", "Thread Status", RRDF_FIELD_TYPE_STRING,
                             RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                             RRDF_FIELD_SORT_ASCENDING, NULL, RRDF_FIELD_SUMMARY_COUNT,
                             RRDF_FIELD_FILTER_MULTISELECT,
                             RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY, NULL);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_string(wb, "default_sort_column", "Thread");

    buffer_json_member_add_object(wb, "charts");
    {
        // Load Methods
        buffer_json_member_add_object(wb, "ebpf_load_methods");
        {
            buffer_json_member_add_string(wb, "name", "Load Methods");
            buffer_json_member_add_string(wb, "type", "line");
            buffer_json_member_add_array(wb, "columns");
            {
                // Should I add both?
                // Should I convert total in this chart for thread name?
                buffer_json_add_array_item_string(wb, "Status");
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

    fwrite(buffer_tostring(wb), buffer_strlen(wb), 1, stdout);
    buffer_free(wb);

    pluginsd_function_result_end_to_stdout();
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
        size_t num_words = pluginsd_split_words(buffer, words, PLUGINSD_MAX_WORDS);

        const char *keyword = get_word(words, num_words, 0);

        if(keyword && strcmp(keyword, PLUGINSD_KEYWORD_FUNCTION) == 0) {
            char *transaction = get_word(words, num_words, 1);
            char *timeout_s = get_word(words, num_words, 2);
            char *function = get_word(words, num_words, 3);

            if(!transaction || !*transaction || !timeout_s || !*timeout_s || !function || !*function) {
                error("Received incomplete %s (transaction = '%s', timeout = '%s', function = '%s'). Ignoring it.",
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
            error("Received unknown command: %s", keyword ? keyword : "(unset)");
    }
    return NULL;
}
