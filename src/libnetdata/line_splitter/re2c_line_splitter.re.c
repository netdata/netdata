/*!max:re2c*/

#include <stddef.h>

size_t quoted_strings_splitter_pluginsd_re2c(char *start, char **words, size_t max_words)
{
    size_t count = 0;

    const char *YYCURSOR = start;
    for (;;) {
    /*!re2c
        re2c:define:YYCTYPE = char;
        re2c:yyfill:enable = 0;

        single_quotes_word = ["] [^"]* ["];
        double_quotes_word = ['] [^']* ['];
        unquoted_word = [^= "'\t\n\v\f\r\x00]+;
        whitespace = [= \t\n\v\f\r]+;

        * {
            if (count < max_words)
                words[count] = NULL;
            return count;
        }
        [\x00] {
            if (count < max_words)
                words[count] = NULL;

            return count;
        }
        single_quotes_word | double_quotes_word {
            if (count == max_words)
                return count;

            start++;
            start[YYCURSOR - start - 1] = '\0';
            words[count++] = start;
            continue;
        }
        unquoted_word {
            if (count == max_words)
                return count;

            start[YYCURSOR - start] = '\0';
            words[count++] = start;
            start = (char *) ++YYCURSOR;
            continue;
        }
        whitespace {
            start = (char *) YYCURSOR;
            continue;
        }
    */
    }
}
