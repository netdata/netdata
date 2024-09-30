// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_internals.h"

ssize_t send_to_plugin(const char *txt, PARSER *parser) {
    if(!txt || !*txt || !parser)
        return 0;

#ifdef ENABLE_H2O
    if(parser->h2o_ctx)
        return h2o_stream_write(parser->h2o_ctx, txt, strlen(txt));
#endif

    errno_clear();
    spinlock_lock(&parser->writer.spinlock);
    ssize_t bytes = -1;

    NETDATA_SSL *ssl = parser->ssl_output;
    if(ssl) {

        if(SSL_connection(ssl))
            bytes = netdata_ssl_write(ssl, (void *) txt, strlen(txt));

        else
            netdata_log_error("PLUGINSD: cannot send command (SSL)");

        spinlock_unlock(&parser->writer.spinlock);
        return bytes;
    }

    if(parser->fd_output != -1) {
        bytes = 0;
        ssize_t total = (ssize_t)strlen(txt);
        ssize_t sent;

        do {
            sent = write(parser->fd_output, &txt[bytes], total - bytes);
            if(sent <= 0) {
                netdata_log_error("PLUGINSD: cannot send command (fd)");
                spinlock_unlock(&parser->writer.spinlock);
                return -3;
            }
            bytes += sent;
        }
        while(bytes < total);

        spinlock_unlock(&parser->writer.spinlock);
        return (int)bytes;
    }

    spinlock_unlock(&parser->writer.spinlock);
    netdata_log_error("PLUGINSD: cannot send command (no output socket/pipe/file given to plugins.d parser)");
    return -4;
}

PARSER_RC PLUGINSD_DISABLE_PLUGIN(PARSER *parser, const char *keyword, const char *msg) {
    parser->user.enabled = 0;

    if(keyword && msg) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_INFO,
                     "PLUGINSD: keyword %s: %s", keyword, msg);
    }

    return PARSER_RC_ERROR;
}

void pluginsd_keywords_init(PARSER *parser, PARSER_REPERTOIRE repertoire) {
    parser_init_repertoire(parser, repertoire);

    if (repertoire & (PARSER_INIT_PLUGINSD | PARSER_INIT_STREAMING))
        pluginsd_inflight_functions_init(parser);
}

void parser_destroy(PARSER *parser) {
    if (unlikely(!parser))
        return;

    pluginsd_inflight_functions_cleanup(parser);

    freez(parser);
}


PARSER *parser_init(struct parser_user_object *user, int fd_input, int fd_output,
                    PARSER_INPUT_TYPE flags, void *ssl __maybe_unused) {
    PARSER *parser;

    parser = callocz(1, sizeof(*parser));
    if(user)
        parser->user = *user;
    parser->fd_input = fd_input;
    parser->fd_output = fd_output;
    parser->ssl_output = ssl;
    parser->flags = flags;

    spinlock_init(&parser->writer.spinlock);
    return parser;
}
