#ifndef NETDATA_REGISTRY_PERSON_H
#define NETDATA_REGISTRY_PERSON_H

/**
 * @file registry_person.h
 * @brief File contains data structures and methods of a person in the registry.
 */

#include "registry_internals.h"

// ----------------------------------------------------------------------------
// PERSON structures

/// for each PERSON-URL pair we keep this
struct registry_person_url {
    avl avl;                    ///< binary tree node

    REGISTRY_URL *url;          ///< de-duplicated URL
    REGISTRY_MACHINE *machine;  ///< link the MACHINE of this URL

    uint8_t flags;              ///< REGISTRY_URL_FLAGS_*

    uint32_t first_t;           ///< the first time we saw this
    uint32_t last_t;            ///< the last time we saw this
    uint32_t usages;            ///< how many times this has been accessed

    char machine_name[1];       ///< the name of the machine, as known by the user
                                ///< dynamically allocated to fit properly
};
typedef struct registry_person_url REGISTRY_PERSON_URL; ///< For each PERSON-URL pair we keep this.

/// A person
struct registry_person {
    char guid[GUID_LEN + 1];    ///< the person GUID

    avl_tree person_urls;       ///< dictionary of PERSON_URLs

    uint32_t first_t;           ///< the first time we saw this
    uint32_t last_t;            ///< the last time we saw this
    uint32_t usages;            ///< how many times this has been accessed

    //uint32_t flags;
    //char *email;
};
typedef struct registry_person REGISTRY_PERSON; ///< A person.

/// PERSON_URL

/**
 * Get REGISTRY_PERSON_URL for url of REGISTRY_PERSON.
 *
 * @param p REGISTRY_PERSON to query.
 * @param url of REGISTRY_PERSON
 * @return REGISTRY_PERSON_URL or NULL
 */
extern REGISTRY_PERSON_URL *registry_person_url_index_find(REGISTRY_PERSON *p, const char *url);
/**
 * Add REGISTRY_PERSON_URL to REGISTRY_PERSON.
 *
 * @param p REGISTRY_PERSON to add REGISTRY_PERSON_URL to.
 * @param pu REGISTRY_PERSON_URL to add
 * @return `pu` or REGISTRY_PERSON_URL equal to `pu`
 */
extern REGISTRY_PERSON_URL *registry_person_url_index_add(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) NEVERNULL WARNUNUSED;
/**
 * Delete REGISTRY_PERSON_URL from REGISTRY_PERSON.
 *
 * @param p REGISTRY_PERSON to delete REGISTRY_PERSON_URL from.
 * @param pu REGISTRY_PERSON_URL to delete
 * @return `pu` or NULL if not found.
 */
extern REGISTRY_PERSON_URL *registry_person_url_index_del(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) WARNUNUSED;

/**
 * Create and initialize a new REGISTRY_PRSON_URL.
 *
 * @param p REGISTRY_PERSON to add REGISTRY_PERSON_URL.
 * @param m REGISTRY_MACHINE referenced by `u`.
 * @param u REGISTRY_URL of REGISTRY_PERSON_URL.
 * @param name of REGISTRY_PRSON_URL
 * @param namelen Length of `name`.
 * @param when Creation time.
 * @return new REGISTRY_PERSON_URL
 */
extern REGISTRY_PERSON_URL *registry_person_url_allocate(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when);
/**
 * Reallocate REGISTRY_PRSON_URL.
 *
 * Needed to change the name of a PERSON_URL.
 * \todo ktsaou: Your help needed. Do we want to expose this?
 *
 * @param p REGISTRY_PERSON to add REGISTRY_PERSON_URL.
 * @param m REGISTRY_MACHINE referenced by `u`.
 * @param u REGISTRY_URL of REGISTRY_PERSON_URL.
 * @param name of REGISTRY_PRSON_URL
 * @param namelen Length of `name`.
 * @param when Creation time.
 * @param pu old REGISTRY_PERSON_URL
 * @return reallocated REGISTRY_PERSON_URL
 */
extern REGISTRY_PERSON_URL *registry_person_url_reallocate(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when, REGISTRY_PERSON_URL *pu);

/// PERSON

/**
 * Get REGISTRY_PERSON by guid.
 *
 * @see registry_person_get()
 *
 * @param person_guid REGISTRY_PERSON->guid
 * @return REGISTRY_PERSON_URL or NULL
 */
extern REGISTRY_PERSON *registry_person_find(const char *person_guid);
/**
 * Initialize new REGISTRY_PERSON with guid `person_guid`.
 *
 * @param person_guid REGISTRY_PERSON->guid
 * @param when Creation time.
 * @return new REGISTRY_PERSON_URL
 */
extern REGISTRY_PERSON *registry_person_allocate(const char *person_guid, time_t when);
/**
 * Get REGISTRY_PERSON from registry by guid. If not present, create it. 
 *
 * 1. validate person GUID
 * 2. if it is valid, find it
 * 3. if it is not valid, create a new one
 * 4. return it
 *
 * @see registry_person_get()
 *
 * @param person_guid REGISTRY_PERSON->guid
 * @param when Now.
 * @return REGISTRY_PERSON
 */
extern REGISTRY_PERSON *registry_person_get(const char *person_guid, time_t when);
/**
 * Delete REGISTRY_PERSON from registry.
 *
 * @param p REGISTRY_PERSON to delete.
 */
extern void registry_person_del(REGISTRY_PERSON *p);

/**
 * LINKING PERSON -> PERSON_URL.
 *
 * @param p REGISTRY_PERSON to link to PERSON_URL.
 * @param m REGISTRY_MACHINE referenced by REGISTRY_URL.
 * @param u REGISTRY_URL pointing to REGISTRY_MACHINE.
 * @param name of registry_person_url
 * @param namelen Length of `name`
 * @param when Now.
 * @return a new REGISTRY_PERSON_URL
 */
extern REGISTRY_PERSON_URL *registry_person_link_to_url(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when);
/**
 * Unlink REGISTRY_PERSON from REGISTRY_PERSON_URL.
 *
 * @param p REGISTRY_PERSON to unlink.
 * @param pu REGISTRY_PERSON_URL.
 */
extern void registry_person_unlink_from_url(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu);

#endif //NETDATA_REGISTRY_PERSON_H
