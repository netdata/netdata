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

inline int pluginsd_isspace(char c) {
    switch(c) {
        case ' ':
        case '\t':
        case '\r':
        case '\n':
        case '=':
            return 1;

        default:
            return 0;
    }
}

inline int config_isspace(char c) {
    switch (c) {
        case ' ':
        case '\t':
        case '\r':
        case '\n':
        case ',':
            return 1;

        default:
            return 0;
    }
}

inline int group_by_label_isspace(char c) {
    if(c == ',' || c == '|')
        return 1;

    return 0;
}

inline int dyncfg_id_isspace(char c) {
    if(c == ':')
        return 1;

    return 0;
}

bool isspace_map_pluginsd[256] = {};
bool isspace_map_config[256] = {};
bool isspace_map_group_by_label[256] = {};
bool isspace_dyncfg_id_map[256] = {};

__attribute__((constructor)) void initialize_is_space_arrays(void) {
    for(int c = 0; c < 256 ; c++) {
        isspace_map_pluginsd[c] = pluginsd_isspace((char) c);
        isspace_map_config[c] = config_isspace((char) c);
        isspace_map_group_by_label[c] = group_by_label_isspace((char) c);
        isspace_dyncfg_id_map[c] = dyncfg_id_isspace((char)c);
    }
}
