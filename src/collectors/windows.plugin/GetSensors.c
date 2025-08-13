// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include <sensorsapi.h>
#include <sensors.h>
#include <propidl.h>

int do_GetSensors(int update_every, usec_t dt __maybe_unused)
{
    return 0;
}
