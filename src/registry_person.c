#include "registry_internals.h"

// ----------------------------------------------------------------------------
// PERSON_URL

int person_url_compare(void *a, void *b) {
    register uint32_t hash1 = ((REGISTRY_PERSON_URL *)a)->url->hash;
    register uint32_t hash2 = ((REGISTRY_PERSON_URL *)b)->url->hash;

    if(hash1 < hash2) return -1;
    else if(hash1 > hash2) return 1;
    else return strcmp(((REGISTRY_PERSON_URL *)a)->url->url, ((REGISTRY_PERSON_URL *)b)->url->url);
}

#define registry_person_url_index_add(person, rc) (REGISTRY_PERSON_URL *)avl_insert(&((person)->person_urls), (avl *)(rc))
#define registry_person_url_index_del(person, rc) (REGISTRY_PERSON_URL *)avl_remove(&((person)->person_urls), (avl *)(rc))

REGISTRY_PERSON_URL *registry_person_url_find(REGISTRY_PERSON *p, const char *url) {
    debug(D_REGISTRY, "Registry: registry_person_url_find('%s', '%s')", p->guid, url);

    char buf[sizeof(REGISTRY_URL) + strlen(url)];

    REGISTRY_URL *u = (REGISTRY_URL *)&buf;
    strcpy(u->url, url);
    u->hash = simple_hash(u->url);

    REGISTRY_PERSON_URL tpu = { .url = u };

    REGISTRY_PERSON_URL *pu = (REGISTRY_PERSON_URL *)avl_search(&p->person_urls, (void *)&tpu);
    return pu;
}

REGISTRY_PERSON_URL *registry_person_url_allocate(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when) {
    debug(D_REGISTRY, "registry_person_url_allocate('%s', '%s', '%s'): allocating %zu bytes", p->guid, m->guid, u->url, sizeof(REGISTRY_PERSON_URL) + namelen);

    // protection from too big names
    if(namelen > registry.max_name_length)
        namelen = registry.max_name_length;

    REGISTRY_PERSON_URL *pu = mallocz(sizeof(REGISTRY_PERSON_URL) + namelen);

    // a simple strcpy() should do the job
    // but I prefer to be safe, since the caller specified urllen
    strncpyz(pu->machine_name, name, namelen);

    pu->machine = m;
    pu->first_t = pu->last_t = (uint32_t)when;
    pu->usages = 1;
    pu->url = u;
    pu->flags = REGISTRY_URL_FLAGS_DEFAULT;
    m->links++;

    registry.persons_urls_memory += sizeof(REGISTRY_PERSON_URL) + namelen;

    debug(D_REGISTRY, "registry_person_url_allocate('%s', '%s', '%s'): indexing URL in person", p->guid, m->guid, u->url);
    registry_person_url_index_add(p, pu);

    registry_url_link(u);

    return pu;
}

// this function is needed to change the name of a PERSON_URL
REGISTRY_PERSON_URL *registry_person_url_reallocate(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when, REGISTRY_PERSON_URL *pu) {
    debug(D_REGISTRY, "registry_person_url_reallocate('%s', '%s', '%s'): allocating %zu bytes", p->guid, m->guid, u->url, sizeof(REGISTRY_PERSON_URL) + namelen);

    // remove the existing one from the index
    registry_person_url_index_del(p, pu);
    registry_url_unlink(pu->url);
    pu->machine->links--;
    registry.persons_urls_memory -= sizeof(REGISTRY_PERSON_URL) + strlen(pu->machine_name);

    // allocate a new one
    REGISTRY_PERSON_URL *tpu = registry_person_url_allocate(p, m, u, name, namelen, when);
    tpu->first_t = pu->first_t;
    tpu->last_t = pu->last_t;
    tpu->usages = pu->usages;
    tpu->flags = pu->flags;

    freez(pu);
    return tpu;
}


void registry_person_url_del(REGISTRY_PERSON *p, REGISTRY_PERSON_URL *pu) {
    debug(D_REGISTRY, "Registry: registry_person_url_del('%s', '%s')", p->guid, pu->url->url);
    registry_person_url_index_del(p, pu);
}

// ----------------------------------------------------------------------------
// PERSON

REGISTRY_PERSON *registry_person_find(const char *person_guid) {
    debug(D_REGISTRY, "Registry: registry_person_find('%s')", person_guid);
    return dictionary_get(registry.persons, person_guid);
}

REGISTRY_PERSON *registry_person_allocate(const char *person_guid, time_t when) {
    debug(D_REGISTRY, "Registry: registry_person_allocate('%s'): allocating new person, sizeof(PERSON)=%zu", (person_guid)?person_guid:"", sizeof(REGISTRY_PERSON));

    REGISTRY_PERSON *p = mallocz(sizeof(REGISTRY_PERSON));
    if(!person_guid) {
        for(;;) {
            uuid_t uuid;
            uuid_generate(uuid);
            uuid_unparse_lower(uuid, p->guid);

            debug(D_REGISTRY, "Registry: Checking if the generated person guid '%s' is unique", p->guid);
            if (!dictionary_get(registry.persons, p->guid)) {
                debug(D_REGISTRY, "Registry: generated person guid '%s' is unique", p->guid);
                break;
            }
            else
                info("Registry: generated person guid '%s' found in the registry. Retrying...", p->guid);
        }
    }
    else
        strncpyz(p->guid, person_guid, GUID_LEN);

    debug(D_REGISTRY, "Registry: registry_person_allocate('%s'): creating dictionary of urls", p->guid);
    avl_init(&p->person_urls, person_url_compare);

    p->first_t = p->last_t = (uint32_t)when;
    p->usages = 0;

    registry.persons_memory += sizeof(REGISTRY_PERSON);

    registry.persons_count++;
    dictionary_set(registry.persons, p->guid, p, sizeof(REGISTRY_PERSON));

    return p;
}


// 1. validate person GUID
// 2. if it is valid, find it
// 3. if it is not valid, create a new one
// 4. return it
REGISTRY_PERSON *registry_person_get(const char *person_guid, time_t when) {
    debug(D_REGISTRY, "Registry: registry_person_get('%s'): creating dictionary of urls", person_guid);

    REGISTRY_PERSON *p = NULL;

    if(person_guid && *person_guid) {
        char buf[GUID_LEN + 1];
        // validate it is a GUID
        if(unlikely(registry_regenerate_guid(person_guid, buf) == -1))
            info("Registry: person guid '%s' is not a valid guid. Ignoring it.", person_guid);
        else {
            person_guid = buf;
            p = registry_person_find(person_guid);
        }
    }

    if(!p) p = registry_person_allocate(NULL, when);

    return p;
}

// ----------------------------------------------------------------------------
// LINKING OF OBJECTS

REGISTRY_PERSON_URL *registry_person_link_to_url(REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name, size_t namelen, time_t when) {
    debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): searching for URL in person", p->guid, m->guid, u->url);

    REGISTRY_PERSON_URL *pu = registry_person_url_find(p, u->url);
    if(!pu) {
        debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): not found", p->guid, m->guid, u->url);
        pu = registry_person_url_allocate(p, m, u, name, namelen, when);
        registry.persons_urls_count++;
    }
    else {
        debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): found", p->guid, m->guid, u->url);
        pu->usages++;
        if(likely(pu->last_t < (uint32_t)when)) pu->last_t = (uint32_t)when;

        if(pu->machine != m) {
            REGISTRY_MACHINE_URL *mu = dictionary_get(pu->machine->machine_urls, u->url);
            if(mu) {
                debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): URL switched machines (old was '%s') - expiring it from previous machine.",
                     p->guid, m->guid, u->url, pu->machine->guid);
                mu->flags |= REGISTRY_URL_FLAGS_EXPIRED;
            }
            else {
                debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): URL switched machines (old was '%s') - but the URL is not linked to the old machine.",
                     p->guid, m->guid, u->url, pu->machine->guid);
            }

            pu->machine->links--;
            pu->machine = m;
        }

        if(strcmp(pu->machine_name, name)) {
            // the name of the PERSON_URL has changed !
            pu = registry_person_url_reallocate(p, m, u, name, namelen, when, pu);
        }
    }

    p->usages++;
    if(likely(p->last_t < (uint32_t)when)) p->last_t = (uint32_t)when;

    if(pu->flags & REGISTRY_URL_FLAGS_EXPIRED) {
        debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): accessing an expired URL. Re-enabling URL.", p->guid, m->guid, u->url);
        pu->flags &= ~REGISTRY_URL_FLAGS_EXPIRED;
    }

    return pu;
}

