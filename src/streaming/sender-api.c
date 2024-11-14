// SPDX-License-Identifier: GPL-3.0-or-later

#include "sender-internals.h"

void stream_sender_start_host(RRDHOST *host) {
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

    if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {
        __atomic_store_n(&host->sender->exit.shutdown, true, __ATOMIC_RELAXED);
        host->sender->exit.reason = reason;
    }

    struct pipe_msg msg = host->sender->dispatcher.pollfd;
    sender_unlock(host->sender);

    msg.msg = SENDER_MSG_STOP;
    stream_sender_send_msg_to_dispatcher(host->sender, msg);

    if(wait) {
        sender_lock(host->sender);
        while(host->sender->magic) {
            sender_unlock(host->sender);
            sleep_usec(10 * USEC_PER_MS);
            sender_lock(host->sender);
        }
        sender_unlock(host->sender);
    }
}
