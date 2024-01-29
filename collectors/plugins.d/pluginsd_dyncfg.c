// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_dyncfg.h"


// ----------------------------------------------------------------------------

PARSER_RC pluginsd_config(char **words, size_t num_words, PARSER *parser) {
    RRDHOST *host = pluginsd_require_scope_host(parser, PLUGINSD_KEYWORD_CONFIG);
    if(!host) return PARSER_RC_ERROR;

    size_t i = 1;
    char *id     = get_word(words, num_words, i++);
    char *action = get_word(words, num_words, i++);

    if(strcmp(action, PLUGINSD_KEYWORD_CONFIG_ACTION_CREATE) == 0) {
        char *status_str            = get_word(words, num_words, i++);
        char *type_str              = get_word(words, num_words, i++);
        char *path                  = get_word(words, num_words, i++);
        char *source_type_str       = get_word(words, num_words, i++);
        char *source                = get_word(words, num_words, i++);
        char *supported_cmds_str    = get_word(words, num_words, i++);
        char *view_permissions_str  = get_word(words, num_words, i++);
        char *edit_permissions_str  = get_word(words, num_words, i++);

        DYNCFG_STATUS status = dyncfg_status2id(status_str);
        DYNCFG_TYPE type = dyncfg_type2id(type_str);
        DYNCFG_SOURCE_TYPE source_type = dyncfg_source_type2id(source_type_str);
        DYNCFG_CMDS cmds = dyncfg_cmds2id(supported_cmds_str);
        HTTP_ACCESS view_access = http_access_from_hex(view_permissions_str);
        HTTP_ACCESS edit_access = http_access_from_hex(edit_permissions_str);

        if(!dyncfg_add_low_level(
                host,
                id,
                path,
                status,
                type,
                source_type,
                source,
                cmds,
                0,
                0,
                false,
                view_access,
                edit_access,
                pluginsd_function_execute_cb,
                parser))
            return PARSER_RC_ERROR;
    }
    else if(strcmp(action, PLUGINSD_KEYWORD_CONFIG_ACTION_DELETE) == 0) {
        dyncfg_del_low_level(host, id);
    }
    else if(strcmp(action, PLUGINSD_KEYWORD_CONFIG_ACTION_STATUS) == 0) {
        char *status_str         = get_word(words, num_words, i++);
        dyncfg_status_low_level(host, id, dyncfg_status2id(status_str));
    }
    else
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "DYNCFG: unknown action '%s' received from plugin", action);

    parser->user.data_collections_count++;
    return PARSER_RC_OK;
}

// ----------------------------------------------------------------------------

PARSER_RC pluginsd_dyncfg_noop(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    return PARSER_RC_OK;
}
