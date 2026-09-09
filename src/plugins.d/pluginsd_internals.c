// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_internals.h"

ssize_t send_to_plugin(const char *txt, PARSER *parser, STREAM_TRAFFIC_TYPE type) {
    if(!txt || !*txt || !parser)
        return 0;

    if(parser->send_to_plugin_cb)
        return parser->send_to_plugin_cb(txt, parser->send_to_plugin_data, type);

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
        pluginsd_calls_init(parser);
}

void parser_destroy(PARSER *parser) {
    if (unlikely(!parser))
        return;

    // 1. a RESULT_BEGIN..END span cut short by death: release the stashed
    //    item (forcing a 2xx code to 503 - a truncated stream must not report
    //    success) so the sweep below can deliver it and the dictionary is not
    //    queued-for-destruction forever behind our reference
    pluginsd_calls_release_deferred(parser);

    // 2. mark the transport dead and drain the dispatchers: after this no
    //    execute/cancel/progress/GC holds or can acquire the parser.
    //    Drain-safety (streaming): this runs UNDER rrdhost_receiver_lock
    //    (rrdhost_clear_receiver), which is safe because no dispatcher send
    //    path takes the receiver lock (send_to_child takes only its own
    //    spinlock; the opcode path takes the stream-thread queue) - and it is
    //    LOAD-BEARING the other way too: the remaining child-directed senders
    //    (command-nodeid, stream-path) send under the receiver lock and are
    //    serialized with this drain by that same lock. Moving parser_destroy
    //    out of the receiver lock would expose them to the freed parser; a
    //    future send path taking the receiver lock would deadlock this drain.
    //    Both directions are constraints.
    if(parser->inflight.transport)
        nrpc_transport_mark_dead_and_drain(parser->inflight.transport);

    // 3. destroy the container: pre-destroy drain + the destroy-time 503
    //    sweep answer every caller still waiting on this plugin
    pluginsd_calls_cleanup(parser);

    // 4. free the parser, THEN drop the transport's base ref: survivors
    //    (registry entries, dyncfg nodes, in-flight-call pins) keep holding a valid
    //    but dead transport until they release it; its destructor never
    //    touches the parser
    struct nrpc_transport *transport = parser->inflight.transport;
    parser->inflight.transport = NULL;

    freez(parser);

    if(transport)
        nrpc_transport_owner_release(transport);
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
