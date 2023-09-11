// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_REGISTRY_MACHINE_H
#define NETDATA_REGISTRY_MACHINE_H 1

#include "registry_internals.h"

// ----------------------------------------------------------------------------
// MACHINE structures

// For each MACHINE-URL pair we keep this
struct registry_machine_url {
    STRING *url;                // de-duplicated URL

    uint8_t flags;

    uint32_t first_t;           // the first time we saw this
    uint32_t last_t;            // the last time we saw this
    uint32_t usages;            // how many times this has been accessed

    struct registry_machine_url *prev, *next;
};
typedef struct registry_machine_url REGISTRY_MACHINE_URL;

// A machine
struct registry_machine {
    char guid[GUID_LEN + 1];    // the GUID

    uint32_t links;             // the number of REGISTRY_PERSON_URL linked to this machine

    REGISTRY_MACHINE_URL *machine_urls;   // MACHINE_URL *

    uint32_t first_t;           // the first time we saw this
    uint32_t last_t;            // the last time we saw this
    uint32_t usages;            // how many times this has been accessed
};
typedef struct registry_machine REGISTRY_MACHINE;

REGISTRY_MACHINE *registry_machine_find(const char *machine_guid);
REGISTRY_MACHINE_URL *registry_machine_url_allocate(REGISTRY_MACHINE *m, STRING *u, time_t when);
REGISTRY_MACHINE *registry_machine_allocate(const char *machine_guid, time_t when);
REGISTRY_MACHINE *registry_machine_find_or_create(const char *machine_guid, time_t when, bool is_dummy);
REGISTRY_MACHINE_URL *registry_machine_link_to_url(REGISTRY_MACHINE *m, STRING *url, time_t when);

REGISTRY_MACHINE_URL *registry_machine_url_find(REGISTRY_MACHINE *m, STRING *url);
void registry_machine_url_unlink_from_machine_and_free(REGISTRY_MACHINE *m, REGISTRY_MACHINE_URL *mu);

#endif //NETDATA_REGISTRY_MACHINE_H
