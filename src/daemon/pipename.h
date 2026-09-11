// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DAEMON_PIPENAME_H
#define DAEMON_PIPENAME_H

// The netdatacli socket path: NETDATA_PIPENAME when set, otherwise
// "<run dir>/netdata.pipe". Returns NULL when there is no usable run directory to
// derive it from, so every caller MUST check before using the result.
const char *daemon_pipename(void);

#endif /* DAEMON_PIPENAME_H */
