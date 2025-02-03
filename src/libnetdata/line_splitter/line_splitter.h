// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifndef NETDATA_LINE_SPLITTER_H
#define NETDATA_LINE_SPLITTER_H

#define PLUGINSD_MAX_WORDS 30

struct line_splitter {
    size_t count;                       // counts number of lines
    char *words[PLUGINSD_MAX_WORDS];    // an array of pointers for the words in this line
    size_t num_words;                   // the number of pointers used in this line
};

bool line_splitter_reconstruct_line(BUFFER *wb, void *ptr);

static inline void line_splitter_reset(struct line_splitter *line) {
    line->num_words = 0;
}

int isspace_pluginsd(char c);
int isspace_config(char c);
int isspace_group_by_label(char c);
int isspace_dyncfg_id(char c);
int isspace_whitespace(char c);

extern bool isspace_map_whitespace[256];
extern bool isspace_map_pluginsd[256];
extern bool isspace_map_config[256];
extern bool isspace_map_group_by_label[256];
extern bool isspace_dyncfg_id_map[256];

static ALWAYS_INLINE size_t quoted_strings_splitter(char *str, char **words, size_t max_words, bool *isspace_map) {
    char *s = str, quote = 0;
    size_t i = 0;

    // skip all white space
    while (unlikely(isspace_map[(uint8_t)*s]))
        s++;

    if(unlikely(!*s)) {
        words[i] = NULL;
        return 0;
    }

    // check for quote
    if (unlikely(*s == '\'' || *s == '"')) {
        quote = *s; // remember the quote
        s++;        // skip the quote
    }

    // store the first word
    words[i++] = s;

    // while we have something
    while (likely(*s)) {
        // if it is an escape
        if (unlikely(*s == '\\' && s[1])) {
            // IMPORTANT: support for escaping is incomplete!
            // The backslash character needs to be removed
            // from the parsed string.
            s += 2;
            continue;
        }

            // if it is a quote
        else if (unlikely(*s == quote)) {
            quote = 0;
            *s = ' ';
            continue;
        }

            // if it is a space
        else if (unlikely(quote == 0 && isspace_map[(uint8_t)*s])) {
            // terminate the word
            *s++ = '\0';

            // skip all white space
            while (likely(isspace_map[(uint8_t)*s]))
                s++;

            // check for a quote
            if (unlikely(*s == '\'' || *s == '"')) {
                quote = *s; // remember the quote
                s++;        // skip the quote
            }

            // if we reached the end, stop
            if (unlikely(!*s))
                break;

            // store the next word
            if (likely(i < max_words))
                words[i++] = s;
            else
                break;
        }

            // anything else
        else
            s++;
    }

    if (likely(i < max_words))
        words[i] = NULL;

    return i;
}

#define quoted_strings_splitter_whitespace(str, words, max_words) \
        quoted_strings_splitter(str, words, max_words, isspace_map_whitespace)

#define quoted_strings_splitter_query_group_by_label(str, words, max_words) \
        quoted_strings_splitter(str, words, max_words, isspace_map_group_by_label)

#define quoted_strings_splitter_config(str, words, max_words) \
        quoted_strings_splitter(str, words, max_words, isspace_map_config)

#define quoted_strings_splitter_pluginsd(str, words, max_words) \
        quoted_strings_splitter(str, words, max_words, isspace_map_pluginsd)

#define quoted_strings_splitter_dyncfg_id(str, words, max_words) \
        quoted_strings_splitter(str, words, max_words, isspace_dyncfg_id_map)

static ALWAYS_INLINE char *get_word(char **words, size_t num_words, size_t index) {
    if (unlikely(index >= num_words))
        return NULL;

    return words[index];
}

#endif //NETDATA_LINE_SPLITTER_H
