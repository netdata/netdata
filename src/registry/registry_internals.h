// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_REGISTRY_INTERNALS_H_H
#define NETDATA_REGISTRY_INTERNALS_H_H 1

#include "registry.h"

#define REGISTRY_URL_FLAGS_DEFAULT 0x00
#define REGISTRY_URL_FLAGS_EXPIRED 0x01

#define REGISTRY_DICTIONARY_OPTIONS (DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_SINGLE_THREADED)

#define REGISTRY_VERIFY_COOKIES_GUID "11111111-2222-3333-4444-555555555555"
#define is_dummy_person(person_guid) (strcmp(person_guid, REGISTRY_VERIFY_COOKIES_GUID) == 0)

// ----------------------------------------------------------------------------
// COMMON structures

struct registry {
    int enabled;
    netdata_mutex_t lock;

    // entries counters / statistics
    unsigned long long persons_count;
    unsigned long long machines_count;
    unsigned long long usages_count;
    unsigned long long persons_urls_count;
    unsigned long long machines_urls_count;
    unsigned long long log_count;

    // configuration
    unsigned long long save_registry_every_entries;
    const char *registry_domain;
    const char *hostname;
    const char *registry_to_announce;
    const char *cloud_base_url;
    time_t persons_expiration; // seconds to expire idle persons
    int verify_cookies_redirects;
    int enable_cookies_samesite_secure;

    size_t max_url_length;
    size_t max_name_length;

    // file/path names
    const char *pathname;
    const char *db_filename;
    const char *log_filename;

    // open files
    FILE *log_fp;

    // the database
    DICTIONARY *persons;    // dictionary of REGISTRY_PERSON *,  with key the REGISTRY_PERSON.guid
    DICTIONARY *machines;   // dictionary of REGISTRY_MACHINE *, with key the REGISTRY_MACHINE.guid

    ARAL *persons_aral;
    ARAL *machines_aral;

    ARAL *person_urls_aral;
    ARAL *machine_urls_aral;

    struct aral_statistics aral_stats;
};

#include "registry_machine.h"
#include "registry_person.h"
#include "registry.h"

extern struct registry registry;

// REGISTRY LOW-LEVEL REQUESTS (in registry-internals.c)
REGISTRY_PERSON *registry_request_access(const char *person_guid, char *machine_guid, char *url, char *name, time_t when);
REGISTRY_PERSON *registry_request_delete(const char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);
REGISTRY_MACHINE *registry_request_machine(const char *person_guid, char *request_machine, STRING **hostname);

// REGISTRY LOG (in registry_log.c)
void registry_log(char action, REGISTRY_PERSON *p, REGISTRY_MACHINE *m, STRING *u, const char *name);
int registry_log_open(void);
void registry_log_close(void);
void registry_log_recreate(void);
ssize_t registry_log_load(void);

// REGISTRY DB (in registry_db.c)
int registry_db_save(void);
size_t registry_db_load(void);
int registry_db_should_be_saved(void);

#endif //NETDATA_REGISTRY_INTERNALS_H_H
