#include "common.h"

// ----------------------------------------------------------------------------
// TODO
//
// 1. the default tracking cookie expires in 1 year, but the persons are not
//    removed from the db - this means the database only grows - ideally the
//    database should be cleaned in registry_save() for both on-disk and
//    on-memory entries.
//
//    Cleanup:
//    i. Find all the PERSONs that have expired cookie
//    ii. For each of their PERSON_URLs:
//     - decrement the linked MACHINE links
//     - if the linked MACHINE has no other links, remove the linked MACHINE too
//     - remove the PERSON_URL
//
// 2. add protection to prevent abusing the registry by flooding it with
//    requests to fill the memory and crash it.
//
//    Possible protections:
//    - limit the number of URLs per person
//    - limit the number of URLs per machine
//    - limit the number of persons
//    - limit the number of machines
//    - [DONE] limit the size of URLs
//    - [DONE] limit the size of PERSON_URL names
//    - limit the number of requests that add data to the registry,
//      per client IP per hour
//
// 3. lower memory requirements
//
//    - embed avl structures directly into registry objects, instead of DICTIONARY
//    - store GUIDs in memory as UUID instead of char *
//      (this will also remove the index hash, since UUIDs can be compared directly)
//    - do not track persons using the demo machines only
//      (i.e. start tracking them only when they access a non-demo machine)
//    - [DONE] do not track custom dashboards by default

#define REGISTRY_URL_FLAGS_DEFAULT 0x00
#define REGISTRY_URL_FLAGS_EXPIRED 0x01

#define DICTIONARY_FLAGS DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE | DICTIONARY_FLAG_NAME_LINK_DONT_CLONE

// ----------------------------------------------------------------------------
// COMMON structures

struct registry {
    int enabled;

    char machine_guid[36 + 1];

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
    time_t persons_expiration; // seconds to expire idle persons
    int verify_cookies_redirects;

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
    DICTIONARY *persons;    // dictionary of PERSON *, with key the PERSON.guid
    DICTIONARY *machines;   // dictionary of MACHINE *, with key the MACHINE.guid
    DICTIONARY *urls;       // dictionary of URL *, with key the URL.url

    // concurrency locking
    // we keep different locks for different things
    // so that many tasks can be completed in parallel
    pthread_mutex_t persons_lock;
    pthread_mutex_t machines_lock;
    pthread_mutex_t urls_lock;
    pthread_mutex_t person_urls_lock;
    pthread_mutex_t machine_urls_lock;
    pthread_mutex_t log_lock;
} registry;


// ----------------------------------------------------------------------------
// URL structures
// Save memory by de-duplicating URLs
// so instead of storing URLs all over the place
// we store them here and we keep pointers elsewhere

struct url {
    uint32_t links; // the number of links to this URL - when none is left, we free it
    uint16_t len;   // the length of the URL in bytes
    char url[1];    // the URL - dynamically allocated to more size
};
typedef struct url URL;


// ----------------------------------------------------------------------------
// MACHINE structures

// For each MACHINE-URL pair we keep this
struct machine_url {
    URL *url;                   // de-duplicated URL
//  DICTIONARY *persons;        // dictionary of PERSON *

    uint8_t flags;
    uint32_t first_t;           // the first time we saw this
    uint32_t last_t;            // the last time we saw this
    uint32_t usages;            // how many times this has been accessed
};
typedef struct machine_url MACHINE_URL;

// A machine
struct machine {
    char guid[36 + 1];          // the GUID

    uint32_t links;             // the number of PERSON_URLs linked to this machine

    DICTIONARY *urls;           // MACHINE_URL *

    uint32_t first_t;           // the first time we saw this
    uint32_t last_t;            // the last time we saw this
    uint32_t usages;            // how many times this has been accessed
};
typedef struct machine MACHINE;


// ----------------------------------------------------------------------------
// PERSON structures

// for each PERSON-URL pair we keep this
struct person_url {
    URL *url;                   // de-duplicated URL
    MACHINE *machine;           // link the MACHINE of this URL

    uint8_t flags;
    uint32_t first_t;           // the first time we saw this
    uint32_t last_t;            // the last time we saw this
    uint32_t usages;            // how many times this has been accessed

    char name[1];               // the name of the URL, as known by the user
                                // dynamically allocated to fit properly
};
typedef struct person_url PERSON_URL;

// A person
struct person {
    char guid[36 + 1];          // the person GUID

    DICTIONARY *urls;           // dictionary of PERSON_URL *

    uint32_t first_t;           // the first time we saw this
    uint32_t last_t;            // the last time we saw this
    uint32_t usages;            // how many times this has been accessed
};
typedef struct person PERSON;


// ----------------------------------------------------------------------------
// REGISTRY concurrency locking

static inline void registry_persons_lock(void) {
    pthread_mutex_lock(&registry.persons_lock);
}

static inline void registry_persons_unlock(void) {
    pthread_mutex_unlock(&registry.persons_lock);
}

static inline void registry_machines_lock(void) {
    pthread_mutex_lock(&registry.machines_lock);
}

static inline void registry_machines_unlock(void) {
    pthread_mutex_unlock(&registry.machines_lock);
}

static inline void registry_urls_lock(void) {
    pthread_mutex_lock(&registry.urls_lock);
}

static inline void registry_urls_unlock(void) {
    pthread_mutex_unlock(&registry.urls_lock);
}

// ideally, we should not lock the whole registry for
// updating a person's urls.
// however, to save the memory required for keeping a
// mutex (40 bytes) per person, we do...
static inline void registry_person_urls_lock(PERSON *p) {
    (void)p;
    pthread_mutex_lock(&registry.person_urls_lock);
}

static inline void registry_person_urls_unlock(PERSON *p) {
    (void)p;
    pthread_mutex_unlock(&registry.person_urls_lock);
}

// ideally, we should not lock the whole registry for
// updating a machine's urls.
// however, to save the memory required for keeping a
// mutex (40 bytes) per machine, we do...
static inline void registry_machine_urls_lock(MACHINE *m) {
    (void)m;
    pthread_mutex_lock(&registry.machine_urls_lock);
}

static inline void registry_machine_urls_unlock(MACHINE *m) {
    (void)m;
    pthread_mutex_unlock(&registry.machine_urls_lock);
}

static inline void registry_log_lock(void) {
    pthread_mutex_lock(&registry.log_lock);
}

static inline void registry_log_unlock(void) {
    pthread_mutex_unlock(&registry.log_lock);
}


// ----------------------------------------------------------------------------
// common functions

// parse a GUID and re-generated to be always lower case
// this is used as a protection against the variations of GUIDs
static inline int registry_regenerate_guid(const char *guid, char *result) {
    uuid_t uuid;
    if(unlikely(uuid_parse(guid, uuid) == -1)) {
        info("Registry: GUID '%s' is not a valid GUID.", guid);
        return -1;
    }
    else {
        uuid_unparse_lower(uuid, result);

#ifdef NETDATA_INTERNAL_CHECKS
        if(strcmp(guid, result))
            info("Registry: source GUID '%s' and re-generated GUID '%s' differ!", guid, result);
#endif /* NETDATA_INTERNAL_CHECKS */
    }

    return 0;
}

// make sure the names of the machines / URLs do not contain any tabs
// (which are used as our separator in the database files)
// and are properly trimmed (before and after)
static inline char *registry_fix_machine_name(char *name, size_t *len) {
    char *s = name?name:"";

    // skip leading spaces
    while(*s && isspace(*s)) s++;

    // make sure all spaces are a SPACE
    char *t = s;
    while(*t) {
        if(unlikely(isspace(*t)))
            *t = ' ';

        t++;
    }

    // remove trailing spaces
    while(--t >= s) {
        if(*t == ' ')
            *t = '\0';
        else
            break;
    }
    t++;

    if(likely(len))
        *len = (t - s);

    return s;
}

static inline char *registry_fix_url(char *url, size_t *len) {
    return registry_fix_machine_name(url, len);
}


// ----------------------------------------------------------------------------
// forward definition of functions

extern PERSON *registry_request_access(char *person_guid, char *machine_guid, char *url, char *name, time_t when);
extern PERSON *registry_request_delete(char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);


// ----------------------------------------------------------------------------
// URL

static inline URL *registry_url_allocate_nolock(const char *url, size_t urllen) {
    // protection from too big URLs
    if(urllen > registry.max_url_length)
        urllen = registry.max_url_length;

    debug(D_REGISTRY, "Registry: registry_url_allocate_nolock('%s'): allocating %zu bytes", url, sizeof(URL) + urllen);
    URL *u = mallocz(sizeof(URL) + urllen);

    // a simple strcpy() should do the job
    // but I prefer to be safe, since the caller specified urllen
    u->len = (uint16_t)urllen;
    strncpyz(u->url, url, u->len);
    u->links = 0;

    registry.urls_memory += sizeof(URL) + urllen;

    debug(D_REGISTRY, "Registry: registry_url_allocate_nolock('%s'): indexing it", url);
    dictionary_set(registry.urls, u->url, u, sizeof(URL));

    return u;
}

static inline URL *registry_url_get_nolock(const char *url, size_t urllen) {
    debug(D_REGISTRY, "Registry: registry_url_get_nolock('%s')", url);

    URL *u = dictionary_get(registry.urls, url);
    if(!u) {
        u = registry_url_allocate_nolock(url, urllen);
        registry.urls_count++;
    }

    return u;
}

static inline URL *registry_url_get(const char *url, size_t urllen) {
    debug(D_REGISTRY, "Registry: registry_url_get('%s')", url);

    registry_urls_lock();

    URL *u = registry_url_get_nolock(url, urllen);

    registry_urls_unlock();

    return u;
}

static inline void registry_url_link_nolock(URL *u) {
    u->links++;
    debug(D_REGISTRY, "Registry: registry_url_link_nolock('%s'): URL has now %u links", u->url, u->links);
}

static inline void registry_url_unlink_nolock(URL *u) {
    u->links--;
    if(!u->links) {
        debug(D_REGISTRY, "Registry: registry_url_unlink_nolock('%s'): No more links for this URL", u->url);
        dictionary_del(registry.urls, u->url);
        freez(u);
    }
    else
        debug(D_REGISTRY, "Registry: registry_url_unlink_nolock('%s'): URL has %u links left", u->url, u->links);
}


// ----------------------------------------------------------------------------
// MACHINE

static inline MACHINE *registry_machine_find(const char *machine_guid) {
    debug(D_REGISTRY, "Registry: registry_machine_find('%s')", machine_guid);
    return dictionary_get(registry.machines, machine_guid);
}

static inline MACHINE_URL *registry_machine_url_allocate(MACHINE *m, URL *u, time_t when) {
    debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s'): allocating %zu bytes", m->guid, u->url, sizeof(MACHINE_URL));

    MACHINE_URL *mu = mallocz(sizeof(MACHINE_URL));

    // mu->persons = dictionary_create(DICTIONARY_FLAGS);
    // dictionary_set(mu->persons, p->guid, p, sizeof(PERSON));

    mu->first_t = mu->last_t = (uint32_t)when;
    mu->usages = 1;
    mu->url = u;
    mu->flags = REGISTRY_URL_FLAGS_DEFAULT;

    registry.machines_urls_memory += sizeof(MACHINE_URL);

    debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s'): indexing URL in machine", m->guid, u->url);
    dictionary_set(m->urls, u->url, mu, sizeof(MACHINE_URL));
    registry_url_link_nolock(u);

    return mu;
}

static inline MACHINE *registry_machine_allocate(const char *machine_guid, time_t when) {
    debug(D_REGISTRY, "Registry: registry_machine_allocate('%s'): creating new machine, sizeof(MACHINE)=%zu", machine_guid, sizeof(MACHINE));

    MACHINE *m = mallocz(sizeof(MACHINE));

    strncpyz(m->guid, machine_guid, 36);

    debug(D_REGISTRY, "Registry: registry_machine_allocate('%s'): creating dictionary of urls", machine_guid);
    m->urls = dictionary_create(DICTIONARY_FLAGS);

    m->first_t = m->last_t = (uint32_t)when;
    m->usages = 0;

    registry.machines_memory += sizeof(MACHINE);

    registry.machines_count++;
    dictionary_set(registry.machines, m->guid, m, sizeof(MACHINE));

    return m;
}

// 1. validate machine GUID
// 2. if it is valid, find it or create it and return it
// 3. if it is not valid, return NULL
static inline MACHINE *registry_machine_get(const char *machine_guid, time_t when) {
    MACHINE *m = NULL;

    registry_machines_lock();

    if(likely(machine_guid && *machine_guid)) {
        // validate it is a GUID
        char buf[36 + 1];
        if(unlikely(registry_regenerate_guid(machine_guid, buf) == -1))
            info("Registry: machine guid '%s' is not a valid guid. Ignoring it.", machine_guid);
        else {
            machine_guid = buf;
            m = registry_machine_find(machine_guid);
            if(!m) m = registry_machine_allocate(machine_guid, when);
        }
    }

    registry_machines_unlock();

    return m;
}


// ----------------------------------------------------------------------------
// PERSON

static inline PERSON *registry_person_find(const char *person_guid) {
    debug(D_REGISTRY, "Registry: registry_person_find('%s')", person_guid);
    return dictionary_get(registry.persons, person_guid);
}

static inline PERSON_URL *registry_person_url_allocate(PERSON *p, MACHINE *m, URL *u, char *name, size_t namelen, time_t when) {
    // protection from too big names
    if(namelen > registry.max_name_length)
        namelen = registry.max_name_length;

    debug(D_REGISTRY, "registry_person_url_allocate('%s', '%s', '%s'): allocating %zu bytes", p->guid, m->guid, u->url,
          sizeof(PERSON_URL) + namelen);

    PERSON_URL *pu = mallocz(sizeof(PERSON_URL) + namelen);

    // a simple strcpy() should do the job
    // but I prefer to be safe, since the caller specified urllen
    strncpyz(pu->name, name, namelen);

    pu->machine = m;
    pu->first_t = pu->last_t = when;
    pu->usages = 1;
    pu->url = u;
    pu->flags = REGISTRY_URL_FLAGS_DEFAULT;
    m->links++;

    registry.persons_urls_memory += sizeof(PERSON_URL) + namelen;

    debug(D_REGISTRY, "registry_person_url_allocate('%s', '%s', '%s'): indexing URL in person", p->guid, m->guid, u->url);
    dictionary_set(p->urls, u->url, pu, sizeof(PERSON_URL));
    registry_url_link_nolock(u);

    return pu;
}

static inline PERSON_URL *registry_person_url_reallocate(PERSON *p, MACHINE *m, URL *u, char *name, size_t namelen, time_t when, PERSON_URL *pu) {
    // this function is needed to change the name of a PERSON_URL

    debug(D_REGISTRY, "registry_person_url_reallocate('%s', '%s', '%s'): allocating %zu bytes", p->guid, m->guid, u->url,
          sizeof(PERSON_URL) + namelen);

    PERSON_URL *tpu = registry_person_url_allocate(p, m, u, name, namelen, when);
    tpu->first_t = pu->first_t;
    tpu->last_t = pu->last_t;
    tpu->usages = pu->usages;

    // ok, these are a hack - since the registry_person_url_allocate() is
    // adding these, we have to subtract them
    tpu->machine->links--;
    registry.persons_urls_memory -= sizeof(PERSON_URL) + strlen(pu->name);
    registry_url_unlink_nolock(u);

    freez(pu);

    return tpu;
}

static inline PERSON *registry_person_allocate(const char *person_guid, time_t when) {
    PERSON *p = NULL;

    debug(D_REGISTRY, "Registry: registry_person_allocate('%s'): allocating new person, sizeof(PERSON)=%zu", (person_guid)?person_guid:"", sizeof(PERSON));

    p = mallocz(sizeof(PERSON));

    if(!person_guid) {
        for (; ;) {
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
        strncpyz(p->guid, person_guid, 36);

    debug(D_REGISTRY, "Registry: registry_person_allocate('%s'): creating dictionary of urls", p->guid);
    p->urls = dictionary_create(DICTIONARY_FLAGS);

    p->first_t = p->last_t = when;
    p->usages = 0;

    registry.persons_memory += sizeof(PERSON);

    registry.persons_count++;
    dictionary_set(registry.persons, p->guid, p, sizeof(PERSON));

    return p;
}


// 1. validate person GUID
// 2. if it is valid, find it
// 3. if it is not valid, create a new one
// 4. return it
static inline PERSON *registry_person_get(const char *person_guid, time_t when) {
    PERSON *p = NULL;

    registry_persons_lock();

    if(person_guid && *person_guid) {
        char buf[36 + 1];
        // validate it is a GUID
        if(unlikely(registry_regenerate_guid(person_guid, buf) == -1))
            info("Registry: person guid '%s' is not a valid guid. Ignoring it.", person_guid);
        else {
            person_guid = buf;
            p = registry_person_find(person_guid);
        }
    }

    if(!p) p = registry_person_allocate(NULL, when);

    registry_persons_unlock();

    return p;
}

// ----------------------------------------------------------------------------
// LINKING OF OBJECTS

static inline PERSON_URL *registry_person_link_to_url(PERSON *p, MACHINE *m, URL *u, char *name, size_t namelen, time_t when) {
    debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): searching for URL in person", p->guid, m->guid, u->url);

    registry_person_urls_lock(p);

    PERSON_URL *pu = dictionary_get(p->urls, u->url);
    if(!pu) {
        debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): not found", p->guid, m->guid, u->url);
        pu = registry_person_url_allocate(p, m, u, name, namelen, when);
        registry.persons_urls_count++;
    }
    else {
        debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): found", p->guid, m->guid, u->url);
        pu->usages++;
        if(likely(pu->last_t < (uint32_t)when)) pu->last_t = when;

        if(pu->machine != m) {
            MACHINE_URL *mu = dictionary_get(pu->machine->urls, u->url);
            if(mu) {
                info("registry_person_link_to_url('%s', '%s', '%s'): URL switched machines (old was '%s') - expiring it from previous machine.",
                     p->guid, m->guid, u->url, pu->machine->guid);
                mu->flags |= REGISTRY_URL_FLAGS_EXPIRED;
            }
            else {
                info("registry_person_link_to_url('%s', '%s', '%s'): URL switched machines (old was '%s') - but the URL is not linked to the old machine.",
                     p->guid, m->guid, u->url, pu->machine->guid);
            }

            pu->machine->links--;
            pu->machine = m;
        }

        if(strcmp(pu->name, name)) {
            // the name of the PERSON_URL has changed !
            pu = registry_person_url_reallocate(p, m, u, name, namelen, when, pu);
        }
    }

    p->usages++;
    if(likely(p->last_t < (uint32_t)when)) p->last_t = when;

    if(pu->flags & REGISTRY_URL_FLAGS_EXPIRED) {
        info("registry_person_link_to_url('%s', '%s', '%s'): accessing an expired URL. Re-enabling URL.", p->guid, m->guid, u->url);
        pu->flags &= ~REGISTRY_URL_FLAGS_EXPIRED;
    }

    registry_person_urls_unlock(p);

    return pu;
}

static inline MACHINE_URL *registry_machine_link_to_url(PERSON *p, MACHINE *m, URL *u, time_t when) {
    debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): searching for URL in machine", p->guid, m->guid, u->url);

    registry_machine_urls_lock(m);

    MACHINE_URL *mu = dictionary_get(m->urls, u->url);
    if(!mu) {
        debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): not found", p->guid, m->guid, u->url);
        mu = registry_machine_url_allocate(m, u, when);
        registry.machines_urls_count++;
    }
    else {
        debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): found", p->guid, m->guid, u->url);
        mu->usages++;
        if(likely(mu->last_t < (uint32_t)when)) mu->last_t = when;
    }

    //debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): indexing person in machine", p->guid, m->guid, u->url);
    //dictionary_set(mu->persons, p->guid, p, sizeof(PERSON));

    m->usages++;
    if(likely(m->last_t < (uint32_t)when)) m->last_t = when;

    if(mu->flags & REGISTRY_URL_FLAGS_EXPIRED) {
        info("registry_machine_link_to_url('%s', '%s', '%s'): accessing an expired URL.", p->guid, m->guid, u->url);
        mu->flags &= ~REGISTRY_URL_FLAGS_EXPIRED;
    }

    registry_machine_urls_unlock(m);

    return mu;
}

// ----------------------------------------------------------------------------
// REGISTRY LOG LOAD/SAVE

static inline int registry_should_save_db(void) {
    debug(D_REGISTRY, "log entries %llu, max %llu", registry.log_count, registry.save_registry_every_entries);
    return registry.log_count > registry.save_registry_every_entries;
}

static inline void registry_log(const char action, PERSON *p, MACHINE *m, URL *u, char *name) {
    if(likely(registry.log_fp)) {
        // we lock only if the file is open
        // to allow replaying the log at registry_log_load()
        registry_log_lock();

        if(unlikely(fprintf(registry.log_fp, "%c\t%08x\t%s\t%s\t%s\t%s\n",
                action,
                p->last_t,
                p->guid,
                m->guid,
                name,
                u->url) < 0))
            error("Registry: failed to save log. Registry data may be lost in case of abnormal restart.");

        // we increase the counter even on failures
        // so that the registry will be saved periodically
        registry.log_count++;

        registry_log_unlock();

        // this must be outside the log_lock(), or a deadlock will happen.
        // registry_save() checks the same inside the log_lock, so only
        // one thread will save the db
        if(unlikely(registry_should_save_db()))
            registry_save();
    }
}

static inline int registry_log_open_nolock(void) {
    if(registry.log_fp)
        fclose(registry.log_fp);

    registry.log_fp = fopen(registry.log_filename, "a");

    if(registry.log_fp) {
        if (setvbuf(registry.log_fp, NULL, _IOLBF, 0) != 0)
            error("Cannot set line buffering on registry log file.");
        return 0;
    }

    error("Cannot open registry log file '%s'. Registry data will be lost in case of netdata or server crash.", registry.log_filename);
    return -1;
}

static inline void registry_log_close_nolock(void) {
    if(registry.log_fp) {
        fclose(registry.log_fp);
        registry.log_fp = NULL;
    }
}

static inline void registry_log_recreate_nolock(void) {
    if(registry.log_fp != NULL) {
        registry_log_close_nolock();

        // open it with truncate
        registry.log_fp = fopen(registry.log_filename, "w");
        if(registry.log_fp) fclose(registry.log_fp);
        else error("Cannot truncate registry log '%s'", registry.log_filename);

        registry.log_fp = NULL;

        registry_log_open_nolock();
    }
}

int registry_log_load(void) {
    ssize_t line = -1;

    // closing the log is required here
    // otherwise we will append to it the values we read
    registry_log_close_nolock();

    debug(D_REGISTRY, "Registry: loading active db from: %s", registry.log_filename);
    FILE *fp = fopen(registry.log_filename, "r");
    if(!fp)
        error("Registry: cannot open registry file: %s", registry.log_filename);
    else {
        char *s, buf[4096 + 1];
        line = 0;
        size_t len = 0;

        while ((s = fgets_trim_len(buf, 4096, fp, &len))) {
            line++;

            switch (s[0]) {
                case 'A': // accesses
                case 'D': // deletes

                    // verify it is valid
                    if (unlikely(len < 85 || s[1] != '\t' || s[10] != '\t' || s[47] != '\t' || s[84] != '\t')) {
                        error("Registry: log line %zd is wrong (len = %zu).", line, len);
                        continue;
                    }
                    s[1] = s[10] = s[47] = s[84] = '\0';

                    // get the variables
                    time_t when = strtoul(&s[2], NULL, 16);
                    char *person_guid = &s[11];
                    char *machine_guid = &s[48];
                    char *name = &s[85];

                    // skip the name to find the url
                    char *url = name;
                    while(*url && *url != '\t') url++;
                    if(!*url) {
                        error("Registry: log line %zd does not have a url.", line);
                        continue;
                    }
                    *url++ = '\0';

                    // make sure the person exists
                    // without this, a new person guid will be created
                    PERSON *p = registry_person_find(person_guid);
                    if(!p) p = registry_person_allocate(person_guid, when);

                    if(s[0] == 'A')
                        registry_request_access(p->guid, machine_guid, url, name, when);
                    else
                        registry_request_delete(p->guid, machine_guid, url, name, when);

                    registry.log_count++;
                    break;

                default:
                    error("Registry: ignoring line %zd of filename '%s': %s.", line, registry.log_filename, s);
                    break;
            }
        }

        fclose(fp);
    }

    // open the log again
    registry_log_open_nolock();

    return line;
}


// ----------------------------------------------------------------------------
// REGISTRY REQUESTS

PERSON *registry_request_access(char *person_guid, char *machine_guid, char *url, char *name, time_t when) {
    debug(D_REGISTRY, "registry_request_access('%s', '%s', '%s'): NEW REQUEST", (person_guid)?person_guid:"", machine_guid, url);

    MACHINE *m = registry_machine_get(machine_guid, when);
    if(!m) return NULL;

    // make sure the name is valid
    size_t namelen;
    name = registry_fix_machine_name(name, &namelen);

    size_t urllen;
    url = registry_fix_url(url, &urllen);

    URL *u = registry_url_get(url, urllen);
    PERSON *p = registry_person_get(person_guid, when);

    registry_person_link_to_url(p, m, u, name, namelen, when);
    registry_machine_link_to_url(p, m, u, when);

    registry_log('A', p, m, u, name);

    registry.usages_count++;
    return p;
}

// verify the person, the machine and the URL exist in our DB
PERSON_URL *registry_verify_request(char *person_guid, char *machine_guid, char *url, PERSON **pp, MACHINE **mm) {
    char pbuf[36 + 1], mbuf[36 + 1];

    if(!person_guid || !*person_guid || !machine_guid || !*machine_guid || !url || !*url) {
        info("Registry Request Verification: invalid request! person: '%s', machine '%s', url '%s'", person_guid?person_guid:"UNSET", machine_guid?machine_guid:"UNSET", url?url:"UNSET");
        return NULL;
    }

    // normalize the url
    url = registry_fix_url(url, NULL);

    // make sure the person GUID is valid
    if(registry_regenerate_guid(person_guid, pbuf) == -1) {
        info("Registry Request Verification: invalid person GUID, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    person_guid = pbuf;

    // make sure the machine GUID is valid
    if(registry_regenerate_guid(machine_guid, mbuf) == -1) {
        info("Registry Request Verification: invalid machine GUID, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    machine_guid = mbuf;

    // make sure the machine exists
    MACHINE *m = registry_machine_find(machine_guid);
    if(!m) {
        info("Registry Request Verification: machine not found, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    if(mm) *mm = m;

    // make sure the person exist
    PERSON *p = registry_person_find(person_guid);
    if(!p) {
        info("Registry Request Verification: person not found, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    if(pp) *pp = p;

    PERSON_URL *pu = dictionary_get(p->urls, url);
    if(!pu) {
        info("Registry Request Verification: URL not found for person, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    return pu;
}

PERSON *registry_request_delete(char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when) {
    (void)when;

    PERSON *p = NULL;
    MACHINE *m = NULL;
    PERSON_URL *pu = registry_verify_request(person_guid, machine_guid, url, &p, &m);
    if(!pu || !p || !m) return NULL;

    // normalize the url
    delete_url = registry_fix_url(delete_url, NULL);

    // make sure the user is not deleting the url it uses
    if(!strcmp(delete_url, pu->url->url)) {
        info("Registry Delete Request: delete URL is the one currently accessed, person: '%s', machine '%s', url '%s', delete url '%s'", p->guid, m->guid, pu->url->url, delete_url);
        return NULL;
    }

    registry_person_urls_lock(p);

    PERSON_URL *dpu = dictionary_get(p->urls, delete_url);
    if(!dpu) {
        info("Registry Delete Request: URL not found for person: '%s', machine '%s', url '%s', delete url '%s'", p->guid, m->guid, pu->url->url, delete_url);
        registry_person_urls_unlock(p);
        return NULL;
    }

    registry_log('D', p, m, pu->url, dpu->url->url);

    dictionary_del(p->urls, dpu->url->url);
    registry_url_unlink_nolock(dpu->url);
    freez(dpu);

    registry_person_urls_unlock(p);
    return p;
}


// a structure to pass to the dictionary_get_all() callback handler
struct machine_request_callback_data {
    MACHINE *find_this_machine;
    PERSON_URL *result;
};

// the callback function
// this will be run for every PERSON_URL of this PERSON
int machine_request_callback(void *entry, void *data) {
    PERSON_URL *mypu = (PERSON_URL *)entry;
    struct machine_request_callback_data *myrdata = (struct machine_request_callback_data *)data;

    if(mypu->machine == myrdata->find_this_machine) {
        myrdata->result = mypu;
        return -1; // this will also stop the walk through
    }

    return 0; // continue
}

MACHINE *registry_request_machine(char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when) {
    (void)when;

    char mbuf[36 + 1];

    PERSON *p = NULL;
    MACHINE *m = NULL;
    PERSON_URL *pu = registry_verify_request(person_guid, machine_guid, url, &p, &m);
    if(!pu || !p || !m) return NULL;

    // make sure the machine GUID is valid
    if(registry_regenerate_guid(request_machine, mbuf) == -1) {
        info("Registry Machine URLs request: invalid machine GUID, person: '%s', machine '%s', url '%s', request machine '%s'", p->guid, m->guid, pu->url->url, request_machine);
        return NULL;
    }
    request_machine = mbuf;

    // make sure the machine exists
    m = registry_machine_find(request_machine);
    if(!m) {
        info("Registry Machine URLs request: machine not found, person: '%s', machine '%s', url '%s', request machine '%s'", p->guid, machine_guid, pu->url->url, request_machine);
        return NULL;
    }

    // Verify the user has in the past accessed this machine
    // We will walk through the PERSON_URLs to find the machine
    // linking to our machine

    // a structure to pass to the dictionary_get_all() callback handler
    struct machine_request_callback_data rdata = { m, NULL };

    // request a walk through on the dictionary
    // no need for locking here, the underlying dictionary has its own
    dictionary_get_all(p->urls, machine_request_callback, &rdata);

    if(rdata.result)
        return m;

    return NULL;
}


// ----------------------------------------------------------------------------
// REGISTRY JSON generation

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"

int registry_verify_cookies_redirects(void) {
    return registry.verify_cookies_redirects;
}

const char *registry_to_announce(void) {
    return registry.registry_to_announce;
}

void registry_set_cookie(struct web_client *w, const char *guid) {
    char edate[100];
    time_t et = now_realtime_sec() + registry.persons_expiration;
    struct tm etmbuf, *etm = gmtime_r(&et, &etmbuf);
    strftime(edate, sizeof(edate), "%a, %d %b %Y %H:%M:%S %Z", etm);

    snprintfz(w->cookie1, COOKIE_MAX, NETDATA_REGISTRY_COOKIE_NAME "=%s; Expires=%s", guid, edate);

    if(registry.registry_domain && registry.registry_domain[0])
        snprintfz(w->cookie2, COOKIE_MAX, NETDATA_REGISTRY_COOKIE_NAME "=%s; Domain=%s; Expires=%s", guid, registry.registry_domain, edate);
}

static inline void registry_set_person_cookie(struct web_client *w, PERSON *p) {
    registry_set_cookie(w, p->guid);
}

static inline void registry_json_header(struct web_client *w, const char *action, const char *status) {
    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    buffer_sprintf(w->response.data, "{\n\t\"action\": \"%s\",\n\t\"status\": \"%s\",\n\t\"hostname\": \"%s\",\n\t\"machine_guid\": \"%s\"",
                   action, status, registry.hostname, registry.machine_guid);
}

static inline void registry_json_footer(struct web_client *w) {
    buffer_strcat(w->response.data, "\n}\n");
}

int registry_request_hello_json(struct web_client *w) {
    registry_json_header(w, "hello", REGISTRY_STATUS_OK);

    buffer_sprintf(w->response.data, ",\n\t\"registry\": \"%s\"",
                   registry.registry_to_announce);

    registry_json_footer(w);
    return 200;
}

static inline int registry_json_disabled(struct web_client *w, const char *action) {
    registry_json_header(w, action, REGISTRY_STATUS_DISABLED);

    buffer_sprintf(w->response.data, ",\n\t\"registry\": \"%s\"",
                   registry.registry_to_announce);

    registry_json_footer(w);
    return 200;
}

// structure used be the callbacks below
struct registry_json_walk_person_urls_callback {
    PERSON *p;
    MACHINE *m;
    struct web_client *w;
    int count;
};

// callback for rendering PERSON_URLs
static inline int registry_json_person_url_callback(void *entry, void *data) {
    PERSON_URL *pu = (PERSON_URL *)entry;
    struct registry_json_walk_person_urls_callback *c = (struct registry_json_walk_person_urls_callback *)data;
    struct web_client *w = c->w;

    if(unlikely(c->count++))
        buffer_strcat(w->response.data, ",");

    buffer_sprintf(w->response.data, "\n\t\t[ \"%s\", \"%s\", %u000, %u, \"%s\" ]",
                   pu->machine->guid, pu->url->url, pu->last_t, pu->usages, pu->name);

    return 1;
}

// callback for rendering MACHINE_URLs
static inline int registry_json_machine_url_callback(void *entry, void *data) {
    MACHINE_URL *mu = (MACHINE_URL *)entry;
    struct registry_json_walk_person_urls_callback *c = (struct registry_json_walk_person_urls_callback *)data;
    struct web_client *w = c->w;
    MACHINE *m = c->m;

    if(unlikely(c->count++))
        buffer_strcat(w->response.data, ",");

    buffer_sprintf(w->response.data, "\n\t\t[ \"%s\", \"%s\", %u000, %u ]",
                   m->guid, mu->url->url, mu->last_t, mu->usages);

    return 1;
}


// the main method for registering an access
int registry_request_access_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *name, time_t when) {
    if(!registry.enabled)
        return registry_json_disabled(w, "access");

    PERSON *p = registry_request_access(person_guid, machine_guid, url, name, when);
    if(!p) {
        registry_json_header(w, "access", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return 412;
    }

    // set the cookie
    registry_set_person_cookie(w, p);

    // generate the response
    registry_json_header(w, "access", REGISTRY_STATUS_OK);

    buffer_sprintf(w->response.data, ",\n\t\"person_guid\": \"%s\",\n\t\"urls\": [", p->guid);
    struct registry_json_walk_person_urls_callback c = { p, NULL, w, 0 };
    dictionary_get_all(p->urls, registry_json_person_url_callback, &c);
    buffer_strcat(w->response.data, "\n\t]\n");

    registry_json_footer(w);
    return 200;
}

// the main method for deleting a URL from a person
int registry_request_delete_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when) {
    if(!registry.enabled)
        return registry_json_disabled(w, "delete");

    PERSON *p = registry_request_delete(person_guid, machine_guid, url, delete_url, when);
    if(!p) {
        registry_json_header(w, "delete", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return 412;
    }

    // generate the response
    registry_json_header(w, "delete", REGISTRY_STATUS_OK);
    registry_json_footer(w);
    return 200;
}

// the main method for searching the URLs of a netdata
int registry_request_search_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when) {
    if(!registry.enabled)
        return registry_json_disabled(w, "search");

    MACHINE *m = registry_request_machine(person_guid, machine_guid, url, request_machine, when);
    if(!m) {
        registry_json_header(w, "search", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return 404;
    }

    registry_json_header(w, "search", REGISTRY_STATUS_OK);

    buffer_strcat(w->response.data, ",\n\t\"urls\": [");
    struct registry_json_walk_person_urls_callback c = { NULL, m, w, 0 };
    dictionary_get_all(m->urls, registry_json_machine_url_callback, &c);
    buffer_strcat(w->response.data, "\n\t]\n");

    registry_json_footer(w);
    return 200;
}

// structure used be the callbacks below
struct registry_person_url_callback_verify_machine_exists_data {
    MACHINE *m;
    int count;
};

int registry_person_url_callback_verify_machine_exists(void *entry, void *data) {
    struct registry_person_url_callback_verify_machine_exists_data *d = (struct registry_person_url_callback_verify_machine_exists_data *)data;
    PERSON_URL *pu = (PERSON_URL *)entry;
    MACHINE *m = d->m;

    if(pu->machine == m)
        d->count++;

    return 0;
}

// the main method for switching user identity
int registry_request_switch_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *new_person_guid, time_t when) {
    (void)url;
    (void)when;

    if(!registry.enabled)
        return registry_json_disabled(w, "switch");

    PERSON *op = registry_person_find(person_guid);
    if(!op) {
        registry_json_header(w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return 430;
    }

    PERSON *np = registry_person_find(new_person_guid);
    if(!np) {
        registry_json_header(w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return 431;
    }

    MACHINE *m = registry_machine_find(machine_guid);
    if(!m) {
        registry_json_header(w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return 432;
    }

    struct registry_person_url_callback_verify_machine_exists_data data = { m, 0 };

    // verify the old person has access to this machine
    dictionary_get_all(op->urls, registry_person_url_callback_verify_machine_exists, &data);
    if(!data.count) {
        registry_json_header(w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return 433;
    }

    // verify the new person has access to this machine
    data.count = 0;
    dictionary_get_all(np->urls, registry_person_url_callback_verify_machine_exists, &data);
    if(!data.count) {
        registry_json_header(w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return 434;
    }

    // set the cookie of the new person
    // the user just switched identity
    registry_set_person_cookie(w, np);

    // generate the response
    registry_json_header(w, "switch", REGISTRY_STATUS_OK);
    buffer_sprintf(w->response.data, ",\n\t\"person_guid\": \"%s\"", np->guid);
    registry_json_footer(w);
    return 200;
}


// ----------------------------------------------------------------------------
// REGISTRY THIS MACHINE UNIQUE ID

static inline int is_machine_guid_blacklisted(const char *guid) {
    // these are machine GUIDs that have been included in distribution packages.
    // we blacklist them here, so that the next version of netdata will generate
    // new ones.

    if(!strcmp(guid, "8a795b0c-2311-11e6-8563-000c295076a6")
    || !strcmp(guid, "4aed1458-1c3e-11e6-a53f-000c290fc8f5")
    ) {
        error("Blacklisted machine GUID '%s' found.", guid);
        return 1;
    }

    return 0;
}

char *registry_get_this_machine_guid(void) {
    if(likely(registry.machine_guid[0]))
        return registry.machine_guid;

    // read it from disk
    int fd = open(registry.machine_guid_filename, O_RDONLY);
    if(fd != -1) {
        char buf[36 + 1];
        if(read(fd, buf, 36) != 36)
            error("Failed to read machine GUID from '%s'", registry.machine_guid_filename);
        else {
            buf[36] = '\0';
            if(registry_regenerate_guid(buf, registry.machine_guid) == -1) {
                error("Failed to validate machine GUID '%s' from '%s'. Ignoring it - this might mean this netdata will appear as duplicate in the registry.",
                      buf, registry.machine_guid_filename);

                registry.machine_guid[0] = '\0';
            }
            else if(is_machine_guid_blacklisted(registry.machine_guid))
                registry.machine_guid[0] = '\0';
        }
        close(fd);
    }

    // generate a new one?
    if(!registry.machine_guid[0]) {
        uuid_t uuid;

        uuid_generate_time(uuid);
        uuid_unparse_lower(uuid, registry.machine_guid);
        registry.machine_guid[36] = '\0';

        // save it
        fd = open(registry.machine_guid_filename, O_WRONLY|O_CREAT|O_TRUNC, 444);
        if(fd == -1)
            fatal("Cannot create unique machine id file '%s'. Please fix this.", registry.machine_guid_filename);

        if(write(fd, registry.machine_guid, 36) != 36)
            fatal("Cannot write the unique machine id file '%s'. Please fix this.", registry.machine_guid_filename);

        close(fd);
    }

    setenv("NETDATA_REGISTRY_UNIQUE_ID", registry.machine_guid, 1);

    return registry.machine_guid;
}


// ----------------------------------------------------------------------------
// REGISTRY LOAD/SAVE

int registry_machine_save_url(void *entry, void *file) {
    MACHINE_URL *mu = entry;
    FILE *fp = file;

    debug(D_REGISTRY, "Registry: registry_machine_save_url('%s')", mu->url->url);

    int ret = fprintf(fp, "V\t%08x\t%08x\t%08x\t%02x\t%s\n",
            mu->first_t,
            mu->last_t,
            mu->usages,
            mu->flags,
            mu->url->url
    );

    // error handling is done at registry_save()

    return ret;
}

int registry_machine_save(void *entry, void *file) {
    MACHINE *m = entry;
    FILE *fp = file;

    debug(D_REGISTRY, "Registry: registry_machine_save('%s')", m->guid);

    int ret = fprintf(fp, "M\t%08x\t%08x\t%08x\t%s\n",
            m->first_t,
            m->last_t,
            m->usages,
            m->guid
    );

    if(ret >= 0) {
        int ret2 = dictionary_get_all(m->urls, registry_machine_save_url, fp);
        if(ret2 < 0) return ret2;
        ret += ret2;
    }

    // error handling is done at registry_save()

    return ret;
}

static inline int registry_person_save_url(void *entry, void *file) {
    PERSON_URL *pu = entry;
    FILE *fp = file;

    debug(D_REGISTRY, "Registry: registry_person_save_url('%s')", pu->url->url);

    int ret = fprintf(fp, "U\t%08x\t%08x\t%08x\t%02x\t%s\t%s\t%s\n",
            pu->first_t,
            pu->last_t,
            pu->usages,
            pu->flags,
            pu->machine->guid,
            pu->name,
            pu->url->url
    );

    // error handling is done at registry_save()

    return ret;
}

static inline int registry_person_save(void *entry, void *file) {
    PERSON *p = entry;
    FILE *fp = file;

    debug(D_REGISTRY, "Registry: registry_person_save('%s')", p->guid);

    int ret = fprintf(fp, "P\t%08x\t%08x\t%08x\t%s\n",
            p->first_t,
            p->last_t,
            p->usages,
            p->guid
    );

    if(ret >= 0) {
        int ret2 = dictionary_get_all(p->urls, registry_person_save_url, fp);
        if (ret2 < 0) return ret2;
        ret += ret2;
    }

    // error handling is done at registry_save()

    return ret;
}

int registry_save(void) {
    if(!registry.enabled) return -1;

    // make sure the log is not updated
    registry_log_lock();

    if(unlikely(!registry_should_save_db())) {
        registry_log_unlock();
        return -2;
    }

    error_log_limit_unlimited();

    char tmp_filename[FILENAME_MAX + 1];
    char old_filename[FILENAME_MAX + 1];

    snprintfz(old_filename, FILENAME_MAX, "%s.old", registry.db_filename);
    snprintfz(tmp_filename, FILENAME_MAX, "%s.tmp", registry.db_filename);

    debug(D_REGISTRY, "Registry: Creating file '%s'", tmp_filename);
    FILE *fp = fopen(tmp_filename, "w");
    if(!fp) {
        error("Registry: Cannot create file: %s", tmp_filename);
        registry_log_unlock();
        error_log_limit_reset();
        return -1;
    }

    // dictionary_get_all() has its own locking, so this is safe to do

    debug(D_REGISTRY, "Saving all machines");
    int bytes1 = dictionary_get_all(registry.machines, registry_machine_save, fp);
    if(bytes1 < 0) {
        error("Registry: Cannot save registry machines - return value %d", bytes1);
        fclose(fp);
        registry_log_unlock();
        error_log_limit_reset();
        return bytes1;
    }
    debug(D_REGISTRY, "Registry: saving machines took %d bytes", bytes1);

    debug(D_REGISTRY, "Saving all persons");
    int bytes2 = dictionary_get_all(registry.persons, registry_person_save, fp);
    if(bytes2 < 0) {
        error("Registry: Cannot save registry persons - return value %d", bytes2);
        fclose(fp);
        registry_log_unlock();
        error_log_limit_reset();
        return bytes2;
    }
    debug(D_REGISTRY, "Registry: saving persons took %d bytes", bytes2);

    // save the totals
    fprintf(fp, "T\t%016llx\t%016llx\t%016llx\t%016llx\t%016llx\t%016llx\n",
            registry.persons_count,
            registry.machines_count,
            registry.usages_count + 1, // this is required - it is lost on db rotation
            registry.urls_count,
            registry.persons_urls_count,
            registry.machines_urls_count
    );

    fclose(fp);

    errno = 0;

    // remove the .old db
    debug(D_REGISTRY, "Registry: Removing old db '%s'", old_filename);
    if(unlink(old_filename) == -1 && errno != ENOENT)
        error("Registry: cannot remove old registry file '%s'", old_filename);

    // rename the db to .old
    debug(D_REGISTRY, "Registry: Link current db '%s' to .old: '%s'", registry.db_filename, old_filename);
    if(link(registry.db_filename, old_filename) == -1 && errno != ENOENT)
        error("Registry: cannot move file '%s' to '%s'. Saving registry DB failed!", registry.db_filename, old_filename);

    else {
        // remove the database (it is saved in .old)
        debug(D_REGISTRY, "Registry: removing db '%s'", registry.db_filename);
        if (unlink(registry.db_filename) == -1 && errno != ENOENT)
            error("Registry: cannot remove old registry file '%s'", registry.db_filename);

        // move the .tmp to make it active
        debug(D_REGISTRY, "Registry: linking tmp db '%s' to active db '%s'", tmp_filename, registry.db_filename);
        if (link(tmp_filename, registry.db_filename) == -1) {
            error("Registry: cannot move file '%s' to '%s'. Saving registry DB failed!", tmp_filename,
                  registry.db_filename);

            // move the .old back
            debug(D_REGISTRY, "Registry: linking old db '%s' to active db '%s'", old_filename, registry.db_filename);
            if(link(old_filename, registry.db_filename) == -1)
                error("Registry: cannot move file '%s' to '%s'. Recovering the old registry DB failed!", old_filename, registry.db_filename);
        }
        else {
            debug(D_REGISTRY, "Registry: removing tmp db '%s'", tmp_filename);
            if(unlink(tmp_filename) == -1)
                error("Registry: cannot remove tmp registry file '%s'", tmp_filename);

            // it has been moved successfully
            // discard the current registry log
            registry_log_recreate_nolock();
            registry.log_count = 0;
        }
    }

    // continue operations
    registry_log_unlock();
    error_log_limit_reset();

    return -1;
}

static inline size_t registry_load(void) {
    char *s, buf[4096 + 1];
    PERSON *p = NULL;
    MACHINE *m = NULL;
    URL *u = NULL;
    size_t line = 0;

    debug(D_REGISTRY, "Registry: loading active db from: '%s'", registry.db_filename);
    FILE *fp = fopen(registry.db_filename, "r");
    if(!fp) {
        error("Registry: cannot open registry file: '%s'", registry.db_filename);
        return 0;
    }

    size_t len = 0;
    buf[4096] = '\0';
    while((s = fgets_trim_len(buf, 4096, fp, &len))) {
        line++;

        debug(D_REGISTRY, "Registry: read line %zu to length %zu: %s", line, len, s);
        switch(*s) {
            case 'T': // totals
                if(unlikely(len != 103 || s[1] != '\t' || s[18] != '\t' || s[35] != '\t' || s[52] != '\t' || s[69] != '\t' || s[86] != '\t' || s[103] != '\0')) {
                    error("Registry totals line %zu is wrong (len = %zu).", line, len);
                    continue;
                }
                registry.persons_count = strtoull(&s[2], NULL, 16);
                registry.machines_count = strtoull(&s[19], NULL, 16);
                registry.usages_count = strtoull(&s[36], NULL, 16);
                registry.urls_count = strtoull(&s[53], NULL, 16);
                registry.persons_urls_count = strtoull(&s[70], NULL, 16);
                registry.machines_urls_count = strtoull(&s[87], NULL, 16);
                break;

            case 'P': // person
                m = NULL;
                // verify it is valid
                if(unlikely(len != 65 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[65] != '\0')) {
                    error("Registry person line %zu is wrong (len = %zu).", line, len);
                    continue;
                }

                s[1] = s[10] = s[19] = s[28] = '\0';
                p = registry_person_allocate(&s[29], strtoul(&s[2], NULL, 16));
                p->last_t = strtoul(&s[11], NULL, 16);
                p->usages = strtoul(&s[20], NULL, 16);
                debug(D_REGISTRY, "Registry loaded person '%s', first: %u, last: %u, usages: %u", p->guid, p->first_t, p->last_t, p->usages);
                break;

            case 'M': // machine
                p = NULL;
                // verify it is valid
                if(unlikely(len != 65 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[65] != '\0')) {
                    error("Registry person line %zu is wrong (len = %zu).", line, len);
                    continue;
                }

                s[1] = s[10] = s[19] = s[28] = '\0';
                m = registry_machine_allocate(&s[29], strtoul(&s[2], NULL, 16));
                m->last_t = strtoul(&s[11], NULL, 16);
                m->usages = strtoul(&s[20], NULL, 16);
                debug(D_REGISTRY, "Registry loaded machine '%s', first: %u, last: %u, usages: %u", m->guid, m->first_t, m->last_t, m->usages);
                break;

            case 'U': // person URL
                if(unlikely(!p)) {
                    error("Registry: ignoring line %zu, no person loaded: %s", line, s);
                    continue;
                }

                // verify it is valid
                if(len < 69 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[31] != '\t' || s[68] != '\t') {
                    error("Registry person URL line %zu is wrong (len = %zu).", line, len);
                    continue;
                }

                s[1] = s[10] = s[19] = s[28] = s[31] = s[68] = '\0';

                // skip the name to find the url
                char *url = &s[69];
                while(*url && *url != '\t') url++;
                if(!*url) {
                    error("Registry person URL line %zu does not have a url.", line);
                    continue;
                }
                *url++ = '\0';

                // u = registry_url_allocate_nolock(url, strlen(url));
                u = registry_url_get_nolock(url, strlen(url));

                time_t first_t = strtoul(&s[2], NULL, 16);

                m = registry_machine_find(&s[32]);
                if(!m) m = registry_machine_allocate(&s[32], first_t);

                PERSON_URL *pu = registry_person_url_allocate(p, m, u, &s[69], strlen(&s[69]), first_t);
                pu->last_t = strtoul(&s[11], NULL, 16);
                pu->usages = strtoul(&s[20], NULL, 16);
                pu->flags = strtoul(&s[29], NULL, 16);
                debug(D_REGISTRY, "Registry loaded person URL '%s' with name '%s' of machine '%s', first: %u, last: %u, usages: %u, flags: %02x", u->url, pu->name, m->guid, pu->first_t, pu->last_t, pu->usages, pu->flags);
                break;

            case 'V': // machine URL
                if(unlikely(!m)) {
                    error("Registry: ignoring line %zu, no machine loaded: %s", line, s);
                    continue;
                }

                // verify it is valid
                if(len < 32 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[31] != '\t') {
                    error("Registry person URL line %zu is wrong (len = %zu).", line, len);
                    continue;
                }

                s[1] = s[10] = s[19] = s[28] = s[31] = '\0';
                // u = registry_url_allocate_nolock(&s[32], strlen(&s[32]));
                u = registry_url_get_nolock(&s[32], strlen(&s[32]));

                MACHINE_URL *mu = registry_machine_url_allocate(m, u, strtoul(&s[2], NULL, 16));
                mu->last_t = strtoul(&s[11], NULL, 16);
                mu->usages = strtoul(&s[20], NULL, 16);
                mu->flags = strtoul(&s[29], NULL, 16);
                debug(D_REGISTRY, "Registry loaded machine URL '%s', machine '%s', first: %u, last: %u, usages: %u, flags: %02x", u->url, m->guid, mu->first_t, mu->last_t, mu->usages, mu->flags);
                break;

            default:
                error("Registry: ignoring line %zu of filename '%s': %s.", line, registry.db_filename, s);
                break;
        }
    }
    fclose(fp);

    return line;
}

// ----------------------------------------------------------------------------
// REGISTRY

int registry_init(void) {
    char filename[FILENAME_MAX + 1];

    // registry enabled?
    registry.enabled = config_get_boolean("registry", "enabled", 0);

    // pathnames
    registry.pathname = config_get("registry", "registry db directory", VARLIB_DIR "/registry");
    if(mkdir(registry.pathname, 0770) == -1 && errno != EEXIST)
        fatal("Cannot create directory '%s'.", registry.pathname);

    // filenames
    snprintfz(filename, FILENAME_MAX, "%s/netdata.public.unique.id", registry.pathname);
    registry.machine_guid_filename = config_get("registry", "netdata unique id file", filename);
    registry_get_this_machine_guid();

    snprintfz(filename, FILENAME_MAX, "%s/registry.db", registry.pathname);
    registry.db_filename = config_get("registry", "registry db file", filename);

    snprintfz(filename, FILENAME_MAX, "%s/registry-log.db", registry.pathname);
    registry.log_filename = config_get("registry", "registry log file", filename);

    // configuration options
    registry.save_registry_every_entries = config_get_number("registry", "registry save db every new entries", 1000000);
    registry.persons_expiration = config_get_number("registry", "registry expire idle persons days", 365) * 86400;
    registry.registry_domain = config_get("registry", "registry domain", "");
    registry.registry_to_announce = config_get("registry", "registry to announce", "https://registry.my-netdata.io");
    registry.hostname = config_get("registry", "registry hostname", config_get("global", "hostname", localhost.hostname));
    registry.verify_cookies_redirects = config_get_boolean("registry", "verify browser cookies support", 1);

    setenv("NETDATA_REGISTRY_HOSTNAME", registry.hostname, 1);
    setenv("NETDATA_REGISTRY_URL", registry.registry_to_announce, 1);

    registry.max_url_length = config_get_number("registry", "max URL length", 1024);
    if(registry.max_url_length < 10) {
        registry.max_url_length = 10;
        config_set_number("registry", "max URL length", registry.max_url_length);
    }

    registry.max_name_length = config_get_number("registry", "max URL name length", 50);
    if(registry.max_name_length < 10) {
        registry.max_name_length = 10;
        config_set_number("registry", "max URL name length", registry.max_name_length);
    }

    // initialize entries counters
    registry.persons_count = 0;
    registry.machines_count = 0;
    registry.usages_count = 0;
    registry.urls_count = 0;
    registry.persons_urls_count = 0;
    registry.machines_urls_count = 0;

    // initialize memory counters
    registry.persons_memory = 0;
    registry.machines_memory = 0;
    registry.urls_memory = 0;
    registry.persons_urls_memory = 0;
    registry.machines_urls_memory = 0;

    // initialize locks
    pthread_mutex_init(&registry.persons_lock, NULL);
    pthread_mutex_init(&registry.machines_lock, NULL);
    pthread_mutex_init(&registry.urls_lock, NULL);
    pthread_mutex_init(&registry.person_urls_lock, NULL);
    pthread_mutex_init(&registry.machine_urls_lock, NULL);

    // create dictionaries
    registry.persons = dictionary_create(DICTIONARY_FLAGS);
    registry.machines = dictionary_create(DICTIONARY_FLAGS);
    registry.urls = dictionary_create(DICTIONARY_FLAGS);

    // load the registry database
    if(registry.enabled) {
        registry_log_open_nolock();
        registry_load();
        registry_log_load();

        if(unlikely(registry_should_save_db()))
            registry_save();
    }

    return 0;
}

void registry_free(void) {
    if(!registry.enabled) return;

    // we need to destroy the dictionaries ourselves
    // since the dictionaries use memory we allocated

    while(registry.persons->values_index.root) {
        PERSON *p = ((NAME_VALUE *)registry.persons->values_index.root)->value;

        // fprintf(stderr, "\nPERSON: '%s', first: %u, last: %u, usages: %u\n", p->guid, p->first_t, p->last_t, p->usages);

        while(p->urls->values_index.root) {
            PERSON_URL *pu = ((NAME_VALUE *)p->urls->values_index.root)->value;

            // fprintf(stderr, "\tURL: '%s', first: %u, last: %u, usages: %u, flags: 0x%02x\n", pu->url->url, pu->first_t, pu->last_t, pu->usages, pu->flags);

            debug(D_REGISTRY, "Registry: deleting url '%s' from person '%s'", pu->url->url, p->guid);
            dictionary_del(p->urls, pu->url->url);

            debug(D_REGISTRY, "Registry: unlinking url '%s' from person", pu->url->url);
            registry_url_unlink_nolock(pu->url);

            debug(D_REGISTRY, "Registry: freeing person url");
            freez(pu);
        }

        debug(D_REGISTRY, "Registry: deleting person '%s' from persons registry", p->guid);
        dictionary_del(registry.persons, p->guid);

        debug(D_REGISTRY, "Registry: destroying URL dictionary of person '%s'", p->guid);
        dictionary_destroy(p->urls);

        debug(D_REGISTRY, "Registry: freeing person '%s'", p->guid);
        freez(p);
    }

    while(registry.machines->values_index.root) {
        MACHINE *m = ((NAME_VALUE *)registry.machines->values_index.root)->value;

        // fprintf(stderr, "\nMACHINE: '%s', first: %u, last: %u, usages: %u\n", m->guid, m->first_t, m->last_t, m->usages);

        while(m->urls->values_index.root) {
            MACHINE_URL *mu = ((NAME_VALUE *)m->urls->values_index.root)->value;

            // fprintf(stderr, "\tURL: '%s', first: %u, last: %u, usages: %u, flags: 0x%02x\n", mu->url->url, mu->first_t, mu->last_t, mu->usages, mu->flags);

            //debug(D_REGISTRY, "Registry: destroying persons dictionary from url '%s'", mu->url->url);
            //dictionary_destroy(mu->persons);

            debug(D_REGISTRY, "Registry: deleting url '%s' from person '%s'", mu->url->url, m->guid);
            dictionary_del(m->urls, mu->url->url);

            debug(D_REGISTRY, "Registry: unlinking url '%s' from machine", mu->url->url);
            registry_url_unlink_nolock(mu->url);

            debug(D_REGISTRY, "Registry: freeing machine url");
            freez(mu);
        }

        debug(D_REGISTRY, "Registry: deleting machine '%s' from machines registry", m->guid);
        dictionary_del(registry.machines, m->guid);

        debug(D_REGISTRY, "Registry: destroying URL dictionary of machine '%s'", m->guid);
        dictionary_destroy(m->urls);

        debug(D_REGISTRY, "Registry: freeing machine '%s'", m->guid);
        freez(m);
    }

    // and free the memory of remaining dictionary structures

    debug(D_REGISTRY, "Registry: destroying persons dictionary");
    dictionary_destroy(registry.persons);

    debug(D_REGISTRY, "Registry: destroying machines dictionary");
    dictionary_destroy(registry.machines);

    debug(D_REGISTRY, "Registry: destroying urls dictionary");
    dictionary_destroy(registry.urls);
}

// ----------------------------------------------------------------------------
// STATISTICS

void registry_statistics(void) {
    if(!registry.enabled) return;

    static RRDSET *sts = NULL, *stc = NULL, *stm = NULL;

    if(!sts) sts = rrdset_find("netdata.registry_sessions");
    if(!sts) {
        sts = rrdset_create("netdata", "registry_sessions", NULL, "registry", NULL, "NetData Registry Sessions", "session", 131000, rrd_update_every, RRDSET_TYPE_LINE);

        rrddim_add(sts, "sessions",  NULL,  1, 1, RRDDIM_ABSOLUTE);
    }
    else rrdset_next(sts);

    rrddim_set(sts, "sessions", registry.usages_count);
    rrdset_done(sts);

    // ------------------------------------------------------------------------

    if(!stc) stc = rrdset_find("netdata.registry_entries");
    if(!stc) {
        stc = rrdset_create("netdata", "registry_entries", NULL, "registry", NULL, "NetData Registry Entries", "entries", 131100, rrd_update_every, RRDSET_TYPE_LINE);

        rrddim_add(stc, "persons",        NULL,  1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(stc, "machines",       NULL,  1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(stc, "urls",           NULL,  1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(stc, "persons_urls",   NULL,  1, 1, RRDDIM_ABSOLUTE);
        rrddim_add(stc, "machines_urls",  NULL,  1, 1, RRDDIM_ABSOLUTE);
    }
    else rrdset_next(stc);

    rrddim_set(stc, "persons",       registry.persons_count);
    rrddim_set(stc, "machines",      registry.machines_count);
    rrddim_set(stc, "urls",          registry.urls_count);
    rrddim_set(stc, "persons_urls",  registry.persons_urls_count);
    rrddim_set(stc, "machines_urls", registry.machines_urls_count);
    rrdset_done(stc);

    // ------------------------------------------------------------------------

    if(!stm) stm = rrdset_find("netdata.registry_mem");
    if(!stm) {
        stm = rrdset_create("netdata", "registry_mem", NULL, "registry", NULL, "NetData Registry Memory", "KB", 131300, rrd_update_every, RRDSET_TYPE_STACKED);

        rrddim_add(stm, "persons",        NULL,  1, 1024, RRDDIM_ABSOLUTE);
        rrddim_add(stm, "machines",       NULL,  1, 1024, RRDDIM_ABSOLUTE);
        rrddim_add(stm, "urls",           NULL,  1, 1024, RRDDIM_ABSOLUTE);
        rrddim_add(stm, "persons_urls",   NULL,  1, 1024, RRDDIM_ABSOLUTE);
        rrddim_add(stm, "machines_urls",  NULL,  1, 1024, RRDDIM_ABSOLUTE);
    }
    else rrdset_next(stm);

    rrddim_set(stm, "persons",       registry.persons_memory + registry.persons_count * sizeof(NAME_VALUE) + sizeof(DICTIONARY));
    rrddim_set(stm, "machines",      registry.machines_memory + registry.machines_count * sizeof(NAME_VALUE) + sizeof(DICTIONARY));
    rrddim_set(stm, "urls",          registry.urls_memory + registry.urls_count * sizeof(NAME_VALUE) + sizeof(DICTIONARY));
    rrddim_set(stm, "persons_urls",  registry.persons_urls_memory + registry.persons_count * sizeof(DICTIONARY) + registry.persons_urls_count * sizeof(NAME_VALUE));
    rrddim_set(stm, "machines_urls", registry.machines_urls_memory + registry.machines_count * sizeof(DICTIONARY) + registry.machines_urls_count * sizeof(NAME_VALUE));
    rrdset_done(stm);
}
