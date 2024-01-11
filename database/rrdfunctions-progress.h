// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_PROGRESS_H
#define NETDATA_RRDFUNCTIONS_PROGRESS_H

#include "rrd.h"

int rrdhost_function_progress(BUFFER *wb, const char *function __maybe_unused);

#endif //NETDATA_RRDFUNCTIONS_PROGRESS_H
