// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_PROGRESS_H
#define NETDATA_FUNCTION_PROGRESS_H

#include "daemon/common.h"

int function_progress(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused);

#endif //NETDATA_FUNCTION_PROGRESS_H
