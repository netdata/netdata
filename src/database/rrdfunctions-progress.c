// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdfunctions-progress.h"

int rrdhost_function_progress(BUFFER *wb, const char *function __maybe_unused) {
    return progress_function_result(wb, rrdhost_hostname(localhost));
}

