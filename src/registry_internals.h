#include "common.h"

#ifndef NETDATA_REGISTRY_INTERNALS_H_H
#define NETDATA_REGISTRY_INTERNALS_H_H

#define REGISTRY_URL_FLAGS_DEFAULT 0x00
#define REGISTRY_URL_FLAGS_EXPIRED 0x01

#define DICTIONARY_FLAGS DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE | DICTIONARY_FLAG_NAME_LINK_DONT_CLONE | DICTIONARY_FLAG_SINGLE_THREADED

// ----------------------------------------------------------------------------
// COMMON structures

/**
 * The Registry
 */
struct registry {
    int enabled; ///< boolean

    char machine_guid[GUID_LEN + 1]; ///< global user ID of the registry

    // entries counters / statistics
    unsigned long long persons_count;       ///< number of entries in `persons`
    unsigned long long machines_count;      ///< number of entries in `machines`
    unsigned long long usages_count;        ///< number of accesses to registry
    unsigned long long urls_count;          ///< number of entries in tree `registry_urls_root_index`
    unsigned long long persons_urls_count;  ///< number of person urls
    unsigned long long machines_urls_count; ///< number of machine urls
    unsigned long long log_count;           ///< size of log

    // memory counters / statistics
    unsigned long long persons_memory;       ///< memory used by `persons`
    unsigned long long machines_memory;      ///< memory used by `machines`
    unsigned long long urls_memory;          ///< memory used by `registry_urls_root_index`
    unsigned long long persons_urls_memory;  ///< memory used by person urls
    unsigned long long machines_urls_memory; ///< memory used by machine urls

    // configuration
    unsigned long long save_registry_every_entries; ///< Number after how many new entries registry should be saved
    char *registry_domain;                          ///< domain of the registry
    char *hostname;                                 ///< hostname of the registry
    char *registry_to_announce;                     ///< registry to announce to the web browser
    time_t persons_expiration;                      ///< seconds to expire idle persons
    int verify_cookies_redirects;                   ///< verify cookies redirects

    size_t max_url_length;  ///< maximum length of urls
    size_t max_name_length; ///< maximum length of names

    // file/path names
    char *pathname;              ///< folder name to store registry files in
    char *db_filename;           ///< filename to store the databese in
    char *log_filename;          ///< file to store the log in
    char *machine_guid_filename; ///< file to store guid of this machine

    FILE *log_fp; ///< open files

    // the database
    DICTIONARY *persons;    ///< dictionary of REGISTRY_PERSON *,  with key the REGISTRY_PERSON.guid
    DICTIONARY *machines;   ///< dictionary of REGISTRY_MACHINE *, with key the REGISTRY_MACHINE.guid

    avl_tree registry_urls_root_index; ///< AVL tree of urls

    pthread_mutex_t lock; ///< mutex for synchronizing.
};

extern int registry_regenerate_guid(const char *guid, char *result);

#include "registry_url.h"
#include "registry_machine.h"
#include "registry_person.h"
#include "registry.h"

extern struct registry registry;

extern char *registry_get_this_machine_guid(void);

// REGISTRY LOW-LEVEL REQUESTS (in registry-internals.c)
extern REGISTRY_PERSON *registry_request_access(char *person_guid, char *machine_guid, char *url, char *name, time_t when);
extern REGISTRY_PERSON *registry_request_delete(char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);
extern REGISTRY_MACHINE *registry_request_machine(char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when);

// REGISTRY LOG (in registry_log.c)
extern void registry_log(const char action, REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name);
extern int registry_log_open(void);
extern void registry_log_close(void);
extern void registry_log_recreate(void);
extern ssize_t registry_log_load(void);

// REGISTRY DB (in registry_db.c)
extern int registry_db_save(void);
extern size_t registry_db_load(void);
extern int registry_db_should_be_saved(void);

#endif //NETDATA_REGISTRY_INTERNALS_H_H
