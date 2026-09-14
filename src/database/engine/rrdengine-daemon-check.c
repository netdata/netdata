// SPDX-License-Identifier: GPL-3.0-or-later

// Compiled into netdata for one purpose: it includes rrdengine-daemon.h together with the daemon
// headers that really declare those symbols, so the compiler checks the two sets of prototypes
// against each other. It contains no code. Deleted together with rrdengine-daemon.h.

#include "rrdengine-daemon.h"

#include "daemon/pulse/pulse-gorilla.h"
#include "web/api/queries/weights.h"
#include "database/contexts/rrdcontext.h"
#include "database/sqlite/sqlite_metadata.h"

typedef int rrdengine_daemon_check_is_not_an_empty_translation_unit;
