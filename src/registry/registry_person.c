// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "registry_internals.h"

// ----------------------------------------------------------------------------
// PERSON_URL INDEX

inline REGISTRY_PERSON_URL *registry_person_url_index_find(REGISTRY_PERSON *p, STRING *url) {
    netdata_log_debug(D_REGISTRY, "Registry: registry_person_url_index_find('%s', '%s')", p->guid, string2str(url));

    REGISTRY_PERSON_URL *pu;
    for(pu = p->person_urls ; pu ;pu = pu->next)
        if(pu->url == url)
            break;

    return pu;
}

inline REGISTRY_PERSON_URL *registry_person_url_index_add(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) {
    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(p->person_urls, pu, prev, next);
    return pu;
}

inline REGISTRY_PERSON_URL *registry_person_url_index_del(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) {
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(p->person_urls, pu, prev, next);
    return pu;
}

// ----------------------------------------------------------------------------
// PERSON_URL

REGISTRY_PERSON_URL *registry_person_url_allocate(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, STRING *url, char *machine_name, size_t machine_name_len, time_t when) {
    netdata_log_debug(D_REGISTRY, "registry_person_url_allocate('%s', '%s', '%s'): allocating %zu bytes", p->guid, m->guid, string2str(url), sizeof(REGISTRY_PERSON_URL) + machine_name_len);

    // protection from too big names
    if(machine_name_len > registry.max_name_length)
        machine_name_len = registry.max_name_length;

    REGISTRY_PERSON_URL *pu = aral_mallocz(registry.person_urls_aral);

    // a simple strcpy() should do the job,
    // but I prefer to be safe, since the caller specified name_len
    pu->machine_name = string_strdupz(machine_name);

    pu->machine = m;
    pu->first_t = pu->last_t = (uint32_t)when;
    pu->usages = 1;
    pu->url = string_dup(url);
    pu->flags = REGISTRY_URL_FLAGS_DEFAULT;
    m->links++;

    netdata_log_debug(D_REGISTRY, "registry_person_url_allocate('%s', '%s', '%s'): indexing URL in person", p->guid, m->guid, string2str(url));
    REGISTRY_PERSON_URL *tpu = registry_person_url_index_add(p, pu);
    if(tpu != pu) {
        netdata_log_error("Registry: Attempted to add duplicate person url '%s' with name '%s' to person '%s'", string2str(url), machine_name, p->guid);
        string_freez(pu->machine_name);
        string_freez(pu->url);
        aral_freez(registry.person_urls_aral, pu);
        pu = tpu;
    }

    return pu;
}

void registry_person_url_deindex_and_free(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) {
    netdata_log_debug(D_REGISTRY, "registry_person_url_deindex_and_free('%s', '%s')", p->guid, string2str(pu->url));

    REGISTRY_PERSON_URL *tpu = registry_person_url_index_del(p, pu);
    if(tpu) {
        string_freez(tpu->machine_name);
        string_freez(tpu->url);
        tpu->machine->links--;
        aral_freez(registry.person_urls_aral, tpu);
    }
}

// this function is needed to change the name of a PERSON_URL
REGISTRY_PERSON_URL *registry_person_url_reallocate(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, STRING *url, char *machine_name, size_t machine_name_len, time_t when, REGISTRY_PERSON_URL *pu) {
    netdata_log_debug(D_REGISTRY, "registry_person_url_reallocate('%s', '%s', '%s'): allocating %zu bytes", p->guid, m->guid, string2str(url), sizeof(REGISTRY_PERSON_URL) + machine_name_len);

    // keep a backup
    REGISTRY_PERSON_URL pu2 = {
            .first_t = pu->first_t,
            .last_t = pu->last_t,
            .usages = pu->usages,
            .flags = pu->flags,
            .machine = pu->machine,
            .machine_name = NULL
    };

    // remove the existing one from the index
    registry_person_url_deindex_and_free(p, pu);
    pu = &pu2;

    // allocate a new one
    REGISTRY_PERSON_URL *tpu = registry_person_url_allocate(p, m, url, machine_name, machine_name_len, when);
    tpu->first_t = pu->first_t;
    tpu->last_t = pu->last_t;
    tpu->usages = pu->usages;
    tpu->flags = pu->flags;

    return tpu;
}


// ----------------------------------------------------------------------------
// PERSON

REGISTRY_PERSON *registry_person_find(const char *person_guid) {
    netdata_log_debug(D_REGISTRY, "Registry: registry_person_find('%s')", person_guid);
    return dictionary_get(registry.persons, person_guid);
}

REGISTRY_PERSON *registry_person_allocate(const char *person_guid, time_t when) {
    netdata_log_debug(D_REGISTRY, "Registry: registry_person_allocate('%s'): allocating new person, sizeof(PERSON)=%zu", (person_guid)?person_guid:"", sizeof(REGISTRY_PERSON));

    REGISTRY_PERSON *p = aral_mallocz(registry.persons_aral);
    if(!person_guid) {
        for(;;) {
            nd_uuid_t uuid;
            uuid_generate(uuid);
            uuid_unparse_lower(uuid, p->guid);

            netdata_log_debug(D_REGISTRY, "Registry: Checking if the generated person guid '%s' is unique", p->guid);
            if (!dictionary_get(registry.persons, p->guid)) {
                netdata_log_debug(D_REGISTRY, "Registry: generated person guid '%s' is unique", p->guid);
                break;
            }
            else
                netdata_log_info("Registry: generated person guid '%s' found in the registry. Retrying...", p->guid);
        }
    }
    else
        strncpyz(p->guid, person_guid, GUID_LEN);

    p->person_urls = NULL;

    p->first_t = p->last_t = (uint32_t)when;
    p->usages = 0;

    registry.persons_count++;
    dictionary_set(registry.persons, p->guid, p, sizeof(REGISTRY_PERSON));

    return p;
}


// 1. validate person GUID
// 2. if it is valid, find it
// 3. if it is not valid, create a new one
// 4. return it
REGISTRY_PERSON *registry_person_find_or_create(const char *person_guid, time_t when, bool is_dummy) {
    netdata_log_debug(D_REGISTRY, "Registry: registry_person_find_or_create('%s'): creating dictionary of urls", person_guid);

    char buf[GUID_LEN + 1];
    REGISTRY_PERSON *p = NULL;

    if(person_guid && *person_guid) {
        // validate it is a GUID
        if(unlikely(regenerate_guid(person_guid, buf) == -1)) {
            netdata_log_info("Registry: person guid '%s' is not a valid guid. Ignoring it.", person_guid);
            person_guid = NULL;
        }
        else {
            person_guid = buf;
            p = registry_person_find(person_guid);
            if(!p && !is_dummy)
                person_guid = NULL;
        }
    }
    else
        person_guid = NULL;

    if(!p) p = registry_person_allocate(person_guid, when);

    return p;
}

// ----------------------------------------------------------------------------
// LINKING OF OBJECTS

REGISTRY_PERSON_URL *registry_person_link_to_url(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, STRING *url, char *machine_name, size_t machine_name_len, time_t when) {
    netdata_log_debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): searching for URL in person", p->guid, m->guid, string2str(url));

    REGISTRY_PERSON_URL *pu = registry_person_url_index_find(p, url);
    if(!pu) {
        netdata_log_debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): not found", p->guid, m->guid, string2str(url));
        pu = registry_person_url_allocate(p, m, url, machine_name, machine_name_len, when);
        registry.persons_urls_count++;
    }
    else {
        netdata_log_debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): found", p->guid, m->guid, string2str(url));
        pu->usages++;
        if(likely(pu->last_t < (uint32_t)when)) pu->last_t = (uint32_t)when;

        if(pu->machine != m) {
            REGISTRY_MACHINE_URL *mu = registry_machine_url_find(pu->machine, url);
            if(mu) {
                netdata_log_debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): URL switched machines (old was '%s') - expiring it from previous machine.",
                      p->guid, m->guid, string2str(url), pu->machine->guid);
                mu->flags |= REGISTRY_URL_FLAGS_EXPIRED;
            }
            else {
                netdata_log_debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): URL switched machines (old was '%s') - but the URL is not linked to the old machine.",
                      p->guid, m->guid, string2str(url), pu->machine->guid);
            }

            pu->machine->links--;
            pu->machine = m;
        }

        if(strcmp(string2str(pu->machine_name), machine_name) != 0) {
            // the name of the PERSON_URL has changed !
            pu = registry_person_url_reallocate(p, m, url, machine_name, machine_name_len, when, pu);
        }
    }

    p->usages++;
    if(likely(p->last_t < (uint32_t)when)) p->last_t = (uint32_t)when;

    if(pu->flags & REGISTRY_URL_FLAGS_EXPIRED) {
        netdata_log_debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): accessing an expired URL. Re-enabling URL.", p->guid, m->guid, string2str(url));
        pu->flags &= ~REGISTRY_URL_FLAGS_EXPIRED;
    }

    return pu;
}

void registry_person_unlink_from_url(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) {
    registry_person_url_deindex_and_free(p, pu);
}
