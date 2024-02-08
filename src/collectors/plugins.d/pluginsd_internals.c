// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_internals.h"

ssize_t send_to_plugin(const char *txt, void *data) {
    PARSER *parser = data;

    if(!txt || !*txt)
        return 0;

#ifdef ENABLE_H2O
    if(parser->h2o_ctx)
        return h2o_stream_write(parser->h2o_ctx, txt, strlen(txt));
#endif

    errno = 0;
    spinlock_lock(&parser->writer.spinlock);
    ssize_t bytes = -1;

#ifdef ENABLE_HTTPS
    NETDATA_SSL *ssl = parser->ssl_output;
    if(ssl) {

        if(SSL_connection(ssl))
            bytes = netdata_ssl_write(ssl, (void *) txt, strlen(txt));

        else
            netdata_log_error("PLUGINSD: cannot send command (SSL)");

        spinlock_unlock(&parser->writer.spinlock);
        return bytes;
    }
#endif

    if(parser->fp_output) {

        bytes = fprintf(parser->fp_output, "%s", txt);
        if(bytes <= 0) {
            netdata_log_error("PLUGINSD: cannot send command (FILE)");
            bytes = -2;
        }
        else
            fflush(parser->fp_output);

        spinlock_unlock(&parser->writer.spinlock);
        return bytes;
    }

    if(parser->fd != -1) {
        bytes = 0;
        ssize_t total = (ssize_t)strlen(txt);
        ssize_t sent;

        do {
            sent = write(parser->fd, &txt[bytes], total - bytes);
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


PARSER *parser_init(struct parser_user_object *user, FILE *fp_input, FILE *fp_output, int fd,
                    PARSER_INPUT_TYPE flags, void *ssl __maybe_unused) {
    PARSER *parser;

    parser = callocz(1, sizeof(*parser));
    if(user)
        parser->user = *user;
    parser->fd = fd;
    parser->fp_input = fp_input;
    parser->fp_output = fp_output;
#ifdef ENABLE_HTTPS
    parser->ssl_output = ssl;
#endif
    parser->flags = flags;

    spinlock_init(&parser->writer.spinlock);
    return parser;
}
