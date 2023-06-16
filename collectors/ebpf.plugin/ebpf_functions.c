// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_functions.h"

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
            "   interval:SECONDS\n"
            "      Sets thread to run for a specific period of time, default 600 seconds.\n"
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
 */
static void ebpf_function_thread_manipulation(const char *transaction,
                                 char *function __maybe_unused,
                                 char *line_buffer __maybe_unused,
                                 int line_max __maybe_unused,
                                 int timeout __maybe_unused)
{
    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    size_t num_words = pluginsd_split_words(function, words, PLUGINSD_MAX_WORDS);
    for(int i = 1; i < PLUGINSD_MAX_WORDS ;i++) {
        const char *keyword = get_word(words, num_words, i);
        if (!keyword)
            break;

        if(strcmp(keyword, "help") == 0) {
            ebpf_function_thread_manipulation_help(transaction);
            return;
        }
    }
}


/*****************************************************************
 *  EBPF FUNCTION THREAD
 *****************************************************************/

/**
 * FUNCTION thread.
 *
 * @param ptr a `ebpf_module_t *`.
 * @return always NULL.
 */
void *ebpf_function_thread(void *ptr)
{
    (void)ptr;
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
                    ebpf_function_thread_manipulation(transaction, function, buffer, PLUGINSD_LINE_MAX + 1, timeout);
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
