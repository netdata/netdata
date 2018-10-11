// SPDX-License-Identifier: GPL-3.0-or-later

#include "../common.h"
#include "registry_internals.h"

// ----------------------------------------------------------------------------
// MACHINE

REGISTRY_MACHINE *registry_machine_find(const char *machine_guid) {
    debug(D_REGISTRY, "Registry: registry_machine_find('%s')", machine_guid);
    return dictionary_get(registry.machines, machine_guid);
}

REGISTRY_MACHINE_URL *registry_machine_url_allocate(REGISTRY_MACHINE *m, REGISTRY_URL *u, time_t when) {
    debug(D_REGISTRY, "registry_machine_url_allocate('%s', '%s'): allocating %zu bytes", m->guid, u->url, sizeof(REGISTRY_MACHINE_URL));

    REGISTRY_MACHINE_URL *mu = mallocz(sizeof(REGISTRY_MACHINE_URL));

    mu->first_t = mu->last_t = (uint32_t)when;
    mu->usages = 1;
    mu->url = u;
    mu->flags = REGISTRY_URL_FLAGS_DEFAULT;

    registry.machines_urls_memory += sizeof(REGISTRY_MACHINE_URL);

    debug(D_REGISTRY, "registry_machine_url_allocate('%s', '%s'): indexing URL in machine", m->guid, u->url);
    dictionary_set(m->machine_urls, u->url, mu, sizeof(REGISTRY_MACHINE_URL));

    registry_url_link(u);

    return mu;
}

REGISTRY_MACHINE *registry_machine_allocate(const char *machine_guid, time_t when) {
    debug(D_REGISTRY, "Registry: registry_machine_allocate('%s'): creating new machine, sizeof(MACHINE)=%zu", machine_guid, sizeof(REGISTRY_MACHINE));

    REGISTRY_MACHINE *m = mallocz(sizeof(REGISTRY_MACHINE));

    strncpyz(m->guid, machine_guid, GUID_LEN);

    debug(D_REGISTRY, "Registry: registry_machine_allocate('%s'): creating dictionary of urls", machine_guid);
    m->machine_urls = dictionary_create(DICTIONARY_FLAGS);

    m->first_t = m->last_t = (uint32_t)when;
    m->usages = 0;

    registry.machines_memory += sizeof(REGISTRY_MACHINE);

    registry.machines_count++;
    dictionary_set(registry.machines, m->guid, m, sizeof(REGISTRY_MACHINE));

    return m;
}

// 1. validate machine GUID
// 2. if it is valid, find it or create it and return it
// 3. if it is not valid, return NULL
REGISTRY_MACHINE *registry_machine_get(const char *machine_guid, time_t when) {
    REGISTRY_MACHINE *m = NULL;

    if(likely(machine_guid && *machine_guid)) {
        // validate it is a GUID
        char buf[GUID_LEN + 1];
        if(unlikely(regenerate_guid(machine_guid, buf) == -1))
            info("Registry: machine guid '%s' is not a valid guid. Ignoring it.", machine_guid);
        else {
            machine_guid = buf;
            m = registry_machine_find(machine_guid);
            if(!m) m = registry_machine_allocate(machine_guid, when);
        }
    }

    return m;
}


// ----------------------------------------------------------------------------
// LINKING OF OBJECTS

REGISTRY_MACHINE_URL *registry_machine_link_to_url(REGISTRY_MACHINE *m, REGISTRY_URL *u, time_t when) {
    debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s'): searching for URL in machine", m->guid, u->url);

    REGISTRY_MACHINE_URL *mu = dictionary_get(m->machine_urls, u->url);
    if(!mu) {
        debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s'): not found", m->guid, u->url);
        mu = registry_machine_url_allocate(m, u, when);
        registry.machines_urls_count++;
    }
    else {
        debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s'): found", m->guid, u->url);
        mu->usages++;
        if(likely(mu->last_t < (uint32_t)when)) mu->last_t = (uint32_t)when;
    }

    m->usages++;
    if(likely(m->last_t < (uint32_t)when)) m->last_t = (uint32_t)when;

    if(mu->flags & REGISTRY_URL_FLAGS_EXPIRED) {
        debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s'): accessing an expired URL.", m->guid, u->url);
        mu->flags &= ~REGISTRY_URL_FLAGS_EXPIRED;
    }

    return mu;
}
