// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include <windows.h>

static int initialize(int update_every)
{
    return 0;
}

int do_GetHardwareInfo(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        if (likely(initialize(update_every))) {
            return -1;
        }

        initialized = true;
    }

    return 0;
}

