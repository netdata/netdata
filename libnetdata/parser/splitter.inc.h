// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifndef CUSTOM_IS_SPACE_FUNC
#error "Need to define CUSTOM_IS_SPACE_FUNC"
#define CUSTOM_IS_SPACE_FUNC(x)
#endif

#ifndef SPLITTER_FUNC
#error "Need to define SPLITTER_FUNC"
#define SPLITTER_FUNC foobar
#endif

static inline size_t SPLITTER_FUNC(char *str, char **words, size_t max_words)
{
    char *s = str, quote = 0;
    size_t i = 0;

    // skip all white space
    while (unlikely(CUSTOM_IS_SPACE_FUNC(*s)))
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
        else if (unlikely(quote == 0 && CUSTOM_IS_SPACE_FUNC(*s))) {
            // terminate the word
            *s++ = '\0';

            // skip all white space
            while (likely(CUSTOM_IS_SPACE_FUNC(*s)))
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

    if (i < max_words)
        words[i] = NULL;

    return i;
}

#undef CUSTOM_IS_SPACE_FUNC
#undef SPLITTER_FUNC
