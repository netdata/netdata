// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "registry_internals.h"

// ----------------------------------------------------------------------------
// MACHINE

REGISTRY_MACHINE *registry_machine_find(const char *machine_guid) {
    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_find('%s')", machine_guid);
    return dictionary_get(registry.machines, machine_guid);
}

REGISTRY_MACHINE_URL *registry_machine_url_find(REGISTRY_MACHINE *m, STRING *url) {
    REGISTRY_MACHINE_URL *mu;

    for(mu = m->machine_urls; mu ;mu = mu->next)
        if(mu->url == url)
            break;

    return mu;
}

void registry_machine_url_unlink_from_machine_and_free(REGISTRY_MACHINE *m, REGISTRY_MACHINE_URL *mu) {
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(m->machine_urls, mu, prev, next);
    string_freez(mu->url);
    aral_freez(registry.machine_urls_aral, mu);
}

REGISTRY_MACHINE_URL *registry_machine_url_allocate(REGISTRY_MACHINE *m, STRING *u, time_t when) {
    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_url_allocate('%s', '%s'): allocating %zu bytes", m->guid, string2str(u), sizeof(REGISTRY_MACHINE_URL));

    REGISTRY_MACHINE_URL *mu = aral_mallocz(registry.machine_urls_aral);

    mu->first_t = mu->last_t = (uint32_t)when;
    mu->usages = 1;
    mu->url = string_dup(u);
    mu->flags = REGISTRY_URL_FLAGS_DEFAULT;

    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_url_allocate('%s', '%s'): indexing URL in machine", m->guid, string2str(u));

    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(m->machine_urls, mu, prev, next);

    return mu;
}

REGISTRY_MACHINE *registry_machine_allocate(const char *machine_guid, time_t when) {
    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_allocate('%s'): creating new machine, sizeof(MACHINE)=%zu", machine_guid, sizeof(REGISTRY_MACHINE));

    REGISTRY_MACHINE *m = aral_mallocz(registry.machines_aral);

    strncpyz(m->guid, machine_guid, GUID_LEN);

    m->machine_urls = NULL;

    m->first_t = m->last_t = (uint32_t)when;
    m->usages = 0;
    m->links = 0;

    registry.machines_count++;

    dictionary_set(registry.machines, m->guid, m, sizeof(REGISTRY_MACHINE));

    return m;
}

// 1. validate machine GUID
// 2. if it is valid, find it or create it and return it
// 3. if it is not valid, return NULL
REGISTRY_MACHINE *registry_machine_find_or_create(const char *machine_guid, time_t when, bool is_dummy __maybe_unused) {
    REGISTRY_MACHINE *m = NULL;

    if(likely(machine_guid && *machine_guid)) {
        // validate it is a GUID
        char buf[GUID_LEN + 1];
        if(unlikely(regenerate_guid(machine_guid, buf) == -1))
            netdata_log_info("REGISTRY: machine guid '%s' is not a valid guid. Ignoring it.", machine_guid);
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

REGISTRY_MACHINE_URL *registry_machine_link_to_url(REGISTRY_MACHINE *m, STRING *url, time_t when) {
    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_link_to_url('%s', '%s'): searching for URL in machine", m->guid, string2str(url));

    REGISTRY_MACHINE_URL *mu = registry_machine_url_find(m, url);
    if(!mu) {
        netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_link_to_url('%s', '%s'): not found", m->guid, string2str(url));
        mu = registry_machine_url_allocate(m, url, when);
        registry.machines_urls_count++;
    }
    else {
        netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_link_to_url('%s', '%s'): found", m->guid, string2str(url));
        mu->usages++;
        if(likely(mu->last_t < (uint32_t)when)) mu->last_t = (uint32_t)when;
    }

    m->usages++;
    if(likely(m->last_t < (uint32_t)when)) m->last_t = (uint32_t)when;

    if(mu->flags & REGISTRY_URL_FLAGS_EXPIRED) {
        netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_link_to_url('%s', '%s'): accessing an expired URL.", m->guid, string2str(url));
        mu->flags &= ~REGISTRY_URL_FLAGS_EXPIRED;
    }

    return mu;
}
