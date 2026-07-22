// SPDX-License-Identifier: GPL-3.0-or-later

// Minimal stubs that let the cgroup label-parser test link the real
// src/database/rrdlabels.c implementation without pulling in the whole rrd
// database. rrdlabels.c references a few symbols owned by other subsystems
// (pulse counters, the rrdset metadata-update path, health pattern helpers).
// The label-parser test never exercises those paths, so no-op stubs and a
// counter-sink global are sufficient and keep the test self-contained.

#include "libnetdata/libnetdata.h"

// Counter sink for rrdlabels memory accounting (normally owned by
// src/daemon/pulse/pulse-dictionary.c).
struct dictionary_stats dictionary_stats_category_rrdlabels = { .name = "labels" };

// aral statistics registration is a no-op for the test (owned by the pulse
// subsystem in the full build).
void pulse_aral_register_statistics(struct aral_statistics *stats, const char *name) {
    (void)stats;
    (void)name;
}

void pulse_aral_unregister_statistics(struct aral_statistics *stats) {
    (void)stats;
}

// Reached only from rrdset_update_rrdlabels(), which the parser test never
// calls. Provide a no-op so the dead path links.
void rrdset_metadata_updated(void *st) {
    (void)st;
}

// Health helper reached only by rrdlabels pattern-matching paths the parser
// test does not use.
void *trim_and_add_key_to_values(void *pa, const char *key, void *input) {
    (void)pa;
    (void)key;
    (void)input;
    return NULL;
}
