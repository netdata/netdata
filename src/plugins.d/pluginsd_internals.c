// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_internals.h"

ssize_t send_to_plugin(const char *txt, PARSER *parser, STREAM_TRAFFIC_TYPE type) {
    if(!txt || !*txt || !parser)
        return 0;

    if(parser->send_to_plugin_cb)
        return parser->send_to_plugin_cb(txt, parser->send_to_plugin_data, type);

#ifdef ENABLE_H2O
    if(parser->h2o_ctx)
        return h2o_stream_write(parser->h2o_ctx, txt, strlen(txt));
#endif

    spinlock_lock(&parser->writer.spinlock);

    ND_SOCK tmp = { .fd = parser->fd_output, };
    const char *destination = "child";
    ND_SOCK *s = parser->sock;  // try the socket
    if(!s) {
        destination = "plugin";
        s = &tmp;            // socket is not there, use the pipe
    }

    if(s->fd != -1) {
        // plugins pipe or socket (with or without SSL)

        size_t total = strlen(txt);
        ssize_t bytes = nd_sock_write_persist(s, txt, total, 100);
        if(bytes < (ssize_t)total) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "PLUGINSD: cannot send command to %s (fd = %d, sent bytes = %zd out of %zu)",
                   destination, s->fd, bytes, total);
            spinlock_unlock(&parser->writer.spinlock);
            return -3;
        }

        spinlock_unlock(&parser->writer.spinlock);
        return (int)bytes;
    }

    spinlock_unlock(&parser->writer.spinlock);
    nd_log(NDLS_DAEMON, NDLP_WARNING,
           "PLUGINSD: cannot send command to %s (probably the receiver got disconnected, since no output descriptor is available)",
           destination);
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
                    PARSER_INPUT_TYPE flags, ND_SOCK *sock) {
    PARSER *parser;

    parser = callocz(1, sizeof(*parser));

    if(user)
        parser->user = *user;

    if(sock) {
        parser->fd_input = sock->fd;
        parser->fd_output = sock->fd;
        parser->sock = sock;
    }
    else {
        parser->fd_input = fd_input;
        parser->fd_output = fd_output;
    }

    parser->flags = flags;

    spinlock_init(&parser->writer.spinlock);
    return parser;
}
