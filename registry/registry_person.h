// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_REGISTRY_PERSON_H
#define NETDATA_REGISTRY_PERSON_H 1

#include "registry_internals.h"

// ----------------------------------------------------------------------------
// PERSON structures

// for each PERSON-URL pair we keep this
struct registry_person_url {
    avl_t avl;                  // binary tree node

    REGISTRY_URL *url;          // de-duplicated URL
    REGISTRY_MACHINE *machine;  // link the MACHINE of this URL

    uint8_t flags;

    uint32_t first_t;           // the first time we saw this
    uint32_t last_t;            // the last time we saw this
    uint32_t usages;            // how many times this has been accessed

    char machine_name[1];       // the name of the machine, as known by the user
    // dynamically allocated to fit properly
};
typedef struct registry_person_url REGISTRY_PERSON_URL;

// A person
struct registry_person {
    char guid[GUID_LEN + 1];    // the person GUID

    avl_tree_type person_urls;  // dictionary of PERSON_URLs

    uint32_t first_t;           // the first time we saw this
    uint32_t last_t;            // the last time we saw this
    uint32_t usages;            // how many times this has been accessed

    //uint32_t flags;
    //char *email;
};
typedef struct registry_person REGISTRY_PERSON;

// PERSON_URL
REGISTRY_PERSON_URL *registry_person_url_index_find(REGISTRY_PERSON *p, const char *url);
REGISTRY_PERSON_URL *registry_person_url_index_add(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) NEVERNULL WARNUNUSED;
REGISTRY_PERSON_URL *registry_person_url_index_del(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) WARNUNUSED;

REGISTRY_PERSON_URL *registry_person_url_allocate(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when);
REGISTRY_PERSON_URL *registry_person_url_reallocate(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when, REGISTRY_PERSON_URL *pu);

// PERSON
REGISTRY_PERSON *registry_person_find(const char *person_guid);
REGISTRY_PERSON *registry_person_allocate(const char *person_guid, time_t when);
REGISTRY_PERSON *registry_person_get(const char *person_guid, time_t when);

// LINKING PERSON -> PERSON_URL
REGISTRY_PERSON_URL *registry_person_link_to_url(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when);
void registry_person_unlink_from_url(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu);

#endif //NETDATA_REGISTRY_PERSON_H
