#include "common.h"

#ifndef NETDATA_REGISTRY_INTERNALS_H_H
#define NETDATA_REGISTRY_INTERNALS_H_H

/**
 * @file registry_internals.h
 * @brief API of the registry for internal use.
 */

#define REGISTRY_URL_FLAGS_DEFAULT 0x00 ///< No special meaning.
#define REGISTRY_URL_FLAGS_EXPIRED 0x01 ///< REGISTRY_URL expired.

#define DICTIONARY_FLAGS DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE | DICTIONARY_FLAG_NAME_LINK_DONT_CLONE | DICTIONARY_FLAG_SINGLE_THREADED ///< Default dictionary flags.

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

    FILE *log_fp; ///< Open log file.

    // the database
    DICTIONARY *persons;    ///< dictionary of REGISTRY_PERSON *,  with key the REGISTRY_PERSON.guid
    DICTIONARY *machines;   ///< dictionary of REGISTRY_MACHINE *, with key the REGISTRY_MACHINE.guid

    avl_tree registry_urls_root_index; ///< AVL tree of urls

    pthread_mutex_t lock; ///< mutex for synchronizing.
};

/**
 * Parse a GUID and re-generated to be always lower case.
 *
 * This is used as a protection against the variations of GUIDs.
 *
 * @param guid to re-generate
 * @param result filled after call
 * @return 0 on success, -1 on error.
 */
extern int registry_regenerate_guid(const char *guid, char *result);

#include "registry_url.h"
#include "registry_machine.h"
#include "registry_person.h"
#include "registry.h"

extern struct registry registry; ///< Global registry.

/**
 * Get GUID of this machine.
 *
 * @return guid of this machine.
 */
extern char *registry_get_this_machine_guid(void);

// REGISTRY LOW-LEVEL REQUESTS (in registry-internals.c)

/**
 * Access the registry with `person_guid`, `machine_guid`, `url` and `name`.
 *
 * @param person_guid of accessing person.
 * @param machine_guid of accessing machine.
 * @param url of accessing machine.
 * @param name of accessing machine.
 * @param when Now.
 * @return corresponding REGIStRY_PERSON.
 */
extern REGISTRY_PERSON *registry_request_access(char *person_guid, char *machine_guid, char *url, char *name, time_t when);
/**
 * Try to delete an URL from REGISTRY_PERSON.
 *
 * @param person_guid of accessing person.
 * @param machine_guid of accessing machine.
 * @param url of accessing machine.
 * @param delete_url to delete.
 * @param when Now.
 * @return maybe changed REGISTRY_PERSON.
 */
extern REGISTRY_PERSON *registry_request_delete(char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);
/**
 * Get REGISTRY_MACHINE for `person_guid`, `machine_guid`, and `url`.
 *
 * @param person_guid of accessing person.
 * @param machine_guid of accessing machine.
 * @param url of accessing machine.
 * @param request_machine to gather.
 * @param when Now.
 * @return REGISTRY_MACHINE.
 */
extern REGISTRY_MACHINE *registry_request_machine(char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when);

// REGISTRY LOG (in registry_log.c)

/**
 * Log a registry action.
 *
 * @param action string
 * @param p person
 * @param m machine
 * @param u url
 * @param name ktsaou: Your help needed.
 */
extern void registry_log(const char action, REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name);
/**
 * (re)open the log file of the registry.
 *
 * @return 0 on success, -1 on error.
 */
extern int registry_log_open(void);
/**
 * Close the log file of the registry.
 */
extern void registry_log_close(void);
/**
 * (re)open and truncate the log file of the registry.
 */
extern void registry_log_recreate(void);
/**
 * ktsaou: Your help needed.
 *
 * @return ktsaou: Your help needed.
 */
extern ssize_t registry_log_load(void);

// REGISTRY DB (in registry_db.c)

/**
 * Save registry to disk.
 *
 * @return ktsaou: Your help needed.
 */
extern int registry_db_save(void);
/**
 * Load registry from disk.
 *
 * @return Lines parsed.
 */
extern size_t registry_db_load(void);
/**
 * Check if registry should be saved. 
 *
 * @return boolean
 */
extern int registry_db_should_be_saved(void);

#endif //NETDATA_REGISTRY_INTERNALS_H_H
