// SPDX-License-Identifier: GPL-3.0-or-later

#include "function-progress.h"

int function_progress(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    return progress_function_result(wb, rrdhost_hostname(localhost));
}

