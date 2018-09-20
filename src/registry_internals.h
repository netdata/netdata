// SPDX-License-Identifier: GPL-3.0+

#ifndef NETDATA_REGISTRY_INTERNALS_H_H
#define NETDATA_REGISTRY_INTERNALS_H_H 1

#define REGISTRY_URL_FLAGS_DEFAULT 0x00
#define REGISTRY_URL_FLAGS_EXPIRED 0x01

#define DICTIONARY_FLAGS (DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE | DICTIONARY_FLAG_NAME_LINK_DONT_CLONE | DICTIONARY_FLAG_SINGLE_THREADED)

// ----------------------------------------------------------------------------
// COMMON structures

struct registry {
    int enabled;

    // entries counters / statistics
    unsigned long long persons_count;
    unsigned long long machines_count;
    unsigned long long usages_count;
    unsigned long long urls_count;
    unsigned long long persons_urls_count;
    unsigned long long machines_urls_count;
    unsigned long long log_count;

    // memory counters / statistics
    unsigned long long persons_memory;
    unsigned long long machines_memory;
    unsigned long long urls_memory;
    unsigned long long persons_urls_memory;
    unsigned long long machines_urls_memory;

    // configuration
    unsigned long long save_registry_every_entries;
    char *registry_domain;
    char *hostname;
    char *registry_to_announce;
    time_t persons_expiration; // seconds to expire idle persons
    int verify_cookies_redirects;

    size_t max_url_length;
    size_t max_name_length;

    // file/path names
    char *pathname;
    char *db_filename;
    char *log_filename;
    char *machine_guid_filename;

    // open files
    FILE *log_fp;

    // the database
    DICTIONARY *persons;    // dictionary of REGISTRY_PERSON *,  with key the REGISTRY_PERSON.guid
    DICTIONARY *machines;   // dictionary of REGISTRY_MACHINE *, with key the REGISTRY_MACHINE.guid

    avl_tree registry_urls_root_index;

    netdata_mutex_t lock;
};

#include "registry_url.h"
#include "registry_machine.h"
#include "registry_person.h"
#include "registry.h"

extern struct registry registry;

// REGISTRY LOW-LEVEL REQUESTS (in registry-internals.c)
extern REGISTRY_PERSON *registry_request_access(char *person_guid, char *machine_guid, char *url, char *name, time_t when);
extern REGISTRY_PERSON *registry_request_delete(char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);
extern REGISTRY_MACHINE *registry_request_machine(char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when);

// REGISTRY LOG (in registry_log.c)
extern void registry_log(char action, REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name);
extern int registry_log_open(void);
extern void registry_log_close(void);
extern void registry_log_recreate(void);
extern ssize_t registry_log_load(void);

// REGISTRY DB (in registry_db.c)
extern int registry_db_save(void);
extern size_t registry_db_load(void);
extern int registry_db_should_be_saved(void);

#endif //NETDATA_REGISTRY_INTERNALS_H_H
