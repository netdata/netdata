// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_REGISTRY_INTERNALS_H_H
#define NETDATA_REGISTRY_INTERNALS_H_H 1

#include "registry.h"

#define REGISTRY_URL_FLAGS_DEFAULT 0x00
#define REGISTRY_URL_FLAGS_EXPIRED 0x01

#define REGISTRY_DICTIONARY_OPTIONS (DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_SINGLE_THREADED)

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
    char *cloud_base_url;
    time_t persons_expiration; // seconds to expire idle persons
    int verify_cookies_redirects;
    int enable_cookies_samesite_secure;

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

    avl_tree_type registry_urls_root_index;

    netdata_mutex_t lock;
};

#include "registry_url.h"
#include "registry_machine.h"
#include "registry_person.h"
#include "registry.h"

extern struct registry registry;

// REGISTRY LOW-LEVEL REQUESTS (in registry-internals.c)
REGISTRY_PERSON *registry_request_access(char *person_guid, char *machine_guid, char *url, char *name, time_t when);
REGISTRY_PERSON *registry_request_delete(char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);
REGISTRY_MACHINE *registry_request_machine(char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when);

// REGISTRY LOG (in registry_log.c)
void registry_log(char action, REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name);
int registry_log_open(void);
void registry_log_close(void);
void registry_log_recreate(void);
ssize_t registry_log_load(void);

// REGISTRY DB (in registry_db.c)
int registry_db_save(void);
size_t registry_db_load(void);
int registry_db_should_be_saved(void);

#endif //NETDATA_REGISTRY_INTERNALS_H_H
