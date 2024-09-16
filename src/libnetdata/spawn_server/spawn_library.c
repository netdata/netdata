// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_library.h"

BUFFER *argv_to_cmdline_buffer(const char **argv) {
    BUFFER *wb = buffer_create(0, NULL);

    for(size_t i = 0; argv[i] ;i++) {
        const char *s = argv[i];
        size_t len = strlen(s);
        buffer_need_bytes(wb, len * 2 + 1);

        bool needs_quotes = false;
        for(const char *c = s; !needs_quotes && *c ; c++) {
            switch(*c) {
                case ' ':
                case '\v':
                case '\t':
                case '\n':
                case '"':
                    needs_quotes = true;
                    break;

                default:
                    break;
            }
        }

        if(needs_quotes && buffer_strlen(wb))
            buffer_strcat(wb, " \"");
        else if(buffer_strlen(wb))
            buffer_putc(wb, ' ');

        for(const char *c = s; *c ; c++) {
            switch(*c) {
                case '"':
                    buffer_putc(wb, '\\');
                    // fall through

                default:
                    buffer_putc(wb, *c);
                    break;
            }
        }

        if(needs_quotes)
            buffer_strcat(wb, "\"");
    }

    return wb;
}
