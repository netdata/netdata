// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

void stream_sender_start_host(RRDHOST *host) {
    internal_fatal(!rrdhost_has_rrdpush_sender_enabled(host),
                   "Host '%s' does not have streaming enabled, but %s() was called",
                   rrdhost_hostname(host), __FUNCTION__);

    stream_sender_start_host_routing(host);
}

void *stream_sender_start_localhost(void *ptr __maybe_unused) {
    if(!localhost) return NULL;
    stream_sender_start_host(localhost);
    return NULL;
}

// Either the receiver lost the connection or the host is being destroyed.
// The sender mutex guards thread creation, any spurious data is wiped on reconnection.
void stream_sender_signal_to_stop_and_wait(RRDHOST *host, STREAM_HANDSHAKE reason, bool wait) {
    if (!host->sender)
        return;

    sender_lock(host->sender);

    if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_ADDED)) {
        __atomic_store_n(&host->sender->exit.shutdown, true, __ATOMIC_RELAXED);
        host->sender->exit.reason = reason;
    }

    struct pipe_msg msg = host->sender->dispatcher.pollfd;
    sender_unlock(host->sender);

    msg.msg = SENDER_MSG_STOP;
    stream_sender_send_msg_to_dispatcher(host->sender, msg);

    while(wait && rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_ADDED))
        sleep_usec(10 * USEC_PER_MS);
}
