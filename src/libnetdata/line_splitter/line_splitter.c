// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"


bool line_splitter_reconstruct_line(BUFFER *wb, void *ptr) {
    struct line_splitter *spl = ptr;
    if(!spl) return false;

    size_t added = 0;
    for(size_t i = 0; i < spl->num_words ;i++) {
        if(i) buffer_fast_strcat(wb, " ", 1);

        buffer_fast_strcat(wb, "'", 1);
        const char *s = get_word(spl->words, spl->num_words, i);
        buffer_strcat(wb, s?s:"");
        buffer_fast_strcat(wb, "'", 1);
        added++;
    }

    return added > 0;
}

inline int isspace_whitespace(char c) {
    switch(c) {
        case ' ':
        case '\t':
        case '\r':
        case '\n':
        case '\f':
        case '\v':
            return 1;

        default:
            return 0;
    }
}

inline int isspace_pluginsd(char c) {
    switch(c) {
        case ' ':
        case '\t':
        case '\r':
        case '\n':
        case '\f':
        case '\v':
        case '=':
            return 1;

        default:
            return 0;
    }
}

inline int isspace_config(char c) {
    switch (c) {
        case ' ':
        case '\t':
        case '\r':
        case '\n':
        case '\f':
        case '\v':
        case ',':
            return 1;

        default:
            return 0;
    }
}

inline int isspace_group_by_label(char c) {
    if(c == ',' || c == '|')
        return 1;

    return 0;
}

inline int isspace_dyncfg_id(char c) {
    if(c == ':')
        return 1;

    return 0;
}

bool isspace_map_whitespace[256] = {};
bool isspace_map_pluginsd[256] = {};
bool isspace_map_config[256] = {};
bool isspace_map_group_by_label[256] = {};
bool isspace_dyncfg_id_map[256] = {};

__attribute__((constructor)) void initialize_is_space_arrays(void) {
    for(int c = 0; c < 256 ; c++) {
        isspace_map_whitespace[c] = isspace_whitespace((char) c);
        isspace_map_pluginsd[c] = isspace_pluginsd((char) c);
        isspace_map_config[c] = isspace_config((char) c);
        isspace_map_group_by_label[c] = isspace_group_by_label((char) c);
        isspace_dyncfg_id_map[c] = isspace_dyncfg_id((char) c);
    }
}
