// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-receiver-internals.h"

char *stream_receiver_program_version_strdupz(RRDHOST *host) {
    rrdhost_receiver_lock(host);
    char *host_version = strdupz(
        host->receiver && host->receiver->program_version ? host->receiver->program_version :
                                                            rrdhost_program_version(host));
    rrdhost_receiver_unlock(host);

    return host_version;
}

bool receiver_has_capability(RRDHOST *host, STREAM_CAPABILITIES caps) {
    rrdhost_receiver_lock(host);
    bool rc = stream_has_capability(host->receiver, caps);
    rrdhost_receiver_unlock(host);
    return rc;
}
