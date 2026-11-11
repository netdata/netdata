// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

typedef enum {
    TELEMETRY_UNKNOWN = 0,
    TELEMETRY_ENABLED,
    TELEMETRY_DISABLED,
} TELEMETRY_STATE;

static TELEMETRY_STATE telemetry_state = TELEMETRY_UNKNOWN;

static TELEMETRY_STATE telemetry_state_detect(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/.opt-out-from-anonymous-statistics", netdata_configured_user_config_dir);

    // the file is there, disable telemetry
    if(access(filename, R_OK) == 0)
        return TELEMETRY_DISABLED;

    // the file is not there, check the environment variable
    const char *s = getenv("DISABLE_TELEMETRY");
    return (!s || !*s) ? TELEMETRY_ENABLED : TELEMETRY_DISABLED;
}

bool telemetry_enabled(void) {
    TELEMETRY_STATE state = __atomic_load_n(&telemetry_state, __ATOMIC_ACQUIRE);
    if(unlikely(state == TELEMETRY_UNKNOWN)) {
        // concurrent first callers detect the same answer, so the race is benign
        state = telemetry_state_detect();
        __atomic_store_n(&telemetry_state, state, __ATOMIC_RELEASE);
    }

    return state == TELEMETRY_ENABLED;
}
