// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_functions.h"

/*****************************************************************
 *  EBPF ERROR FUNCTION
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
 *  EBPF ENABLE FUNCTION
 *****************************************************************/

/**
 * Function enable
 *
 * Enable a specific thread.
 *
 * @param transaction  the transaction id that Netdata sent for this function execution
 * @param function     function name and arguments given to enable threads.
 * @param line_buffer  buffer used to parse args
 * @param line_max     Number of arguments given
 * @param timeout      The function timeout
 */
static void ebpf_function_enable(const char *transaction,
                                 char *function __maybe_unused,
                                 char *line_buffer __maybe_unused,
                                 int line_max __maybe_unused,
                                 int timeout __maybe_unused)
{
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
                if (!strncmp(function, EBPF_FUNCTION_ENABLE, sizeof(EBPF_FUNCTION_ENABLE) - 1))
                    ebpf_function_enable(transaction, function, buffer, PLUGINSD_LINE_MAX + 1, timeout);
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
