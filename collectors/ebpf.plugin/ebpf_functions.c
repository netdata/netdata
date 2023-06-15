// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_functions.h"

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
            }
        }
        else
            error("Received unknown command: %s", keyword ? keyword : "(unset)");
    }
    return NULL;
}
