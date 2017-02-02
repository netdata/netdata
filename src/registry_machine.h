#ifndef NETDATA_REGISTRY_MACHINE_H
#define NETDATA_REGISTRY_MACHINE_H

/**
 * @file registry_machine.h
 * @brief File containing definition and methods for machines in the registriy. 
 */

#include "registry_internals.h"

// ----------------------------------------------------------------------------
// MACHINE structures

/** For each MACHINE-URL pair we keep this */
struct registry_machine_url {
    REGISTRY_URL *url;          ///< de-duplicated URL

    uint8_t flags;              ///< REGISTRY_URL_FLAGS_DEFAULT|REGISTRY_URL_FLAGS_EXPIRED

    uint32_t first_t;           ///< the first time we saw this
    uint32_t last_t;            ///< the last time we saw this
    uint32_t usages;            ///< how many times this has been accessed
};
typedef struct registry_machine_url REGISTRY_MACHINE_URL; ///< For each MACHINE-URL pair we keep this.

/** A machine */
struct registry_machine {
    char guid[GUID_LEN + 1];    ///< the GUID

    uint32_t links;             ///< the number of REGISTRY_PERSON_URL linked to this machine

    DICTIONARY *machine_urls;   ///< MACHINE_URL *

    uint32_t first_t;           ///< the first time we saw this
    uint32_t last_t;            ///< the last time we saw this
    uint32_t usages;            ///< how many times this has been accessed
};
typedef struct registry_machine REGISTRY_MACHINE; ///< A machine.

/**
 * Get REGISTRY_MACHINE for `machine_guid` from the registry.
 *
 * @param machine_guid to query for.
 * @return REGISTRY_MACHINE or NULL
 */
extern REGISTRY_MACHINE *registry_machine_find(const char *machine_guid);
/**
 * Initialize new REGISTRY_MACHINE_URL for REGISTRY_MACHINE and REGISTRY_URL.
 *
 * @param m REGISTRY_MACHINE.
 * @param u REGISTRY_URL.
 * @param when Now.
 * @return new REGISTRY_MACHINE_URL
 */
extern REGISTRY_MACHINE_URL *registry_machine_url_allocate(REGISTRY_MACHINE *m, REGISTRY_URL *u, time_t when);
/**
 * Initialize new REGISTRY_MACHINE for `machine_guid`.
 *
 * @param machine_guid ID of machine.
 * @param when Now.
 * @return new REGISTRY_MACHINE
 */
extern REGISTRY_MACHINE *registry_machine_allocate(const char *machine_guid, time_t when);
/**
 * Get REGISTRY_MACHINE from registry by `machine_guid`. If not present, create it.
 *
 * @param machine_guid ID of machine.
 * @param when Now.
 * @return REGISTRY_MACHINE
 */
extern REGISTRY_MACHINE *registry_machine_get(const char *machine_guid, time_t when);
/**
 * Link REGISTRY_MACHINE to REGISTRY_URL.
 *
 * @param m REGISTRY_MACHINE to link.
 * @param u REGISTRY_URL to link to.
 * @param when Now.
 * @return new REGISTRY_MACHINE_URL
 */
extern REGISTRY_MACHINE_URL *registry_machine_link_to_url(REGISTRY_MACHINE *m, REGISTRY_URL *u, time_t when);

#endif //NETDATA_REGISTRY_MACHINE_H
