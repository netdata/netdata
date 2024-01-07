// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

int systemd_journal_directories_dyncfg_cb(const char *transaction, const char *id, DYNCFG_CMDS cmd, BUFFER *payload, usec_t *stop_monotonic_ut, bool *cancelled, BUFFER *result, void *data) {
    CLEAN_BUFFER *action = buffer_create(100, NULL);
    dyncfg_cmds2buffer(cmd, action);

    nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
           "DYNCFG: transaction '%s', id '%s' cmd '%s', payload: %s",
           transaction, id, buffer_tostring(action), buffer_tostring(payload));

    return HTTP_RESP_OK;
}
