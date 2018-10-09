// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "registry_internals.h"

struct registry registry;

// ----------------------------------------------------------------------------
// common functions

// parse a GUID and re-generated to be always lower case
// this is used as a protection against the variations of GUIDs
int regenerate_guid(const char *guid, char *result) {
    uuid_t uuid;
    if(unlikely(uuid_parse(guid, uuid) == -1)) {
        info("Registry: GUID '%s' is not a valid GUID.", guid);
        return -1;
    }
    else {
        uuid_unparse_lower(uuid, result);

#ifdef NETDATA_INTERNAL_CHECKS
        if(strcmp(guid, result) != 0)
            info("GUID '%s' and re-generated GUID '%s' differ!", guid, result);
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
    size_t l = 0;
    char *s = registry_fix_machine_name(url, &l);

    // protection from too big URLs
    if(l > registry.max_url_length) {
        l = registry.max_url_length;
        s[l] = '\0';
    }

    if(len) *len = l;
    return s;
}


// ----------------------------------------------------------------------------
// HELPERS

// verify the person, the machine and the URL exist in our DB
REGISTRY_PERSON_URL *registry_verify_request(char *person_guid, char *machine_guid, char *url, REGISTRY_PERSON **pp, REGISTRY_MACHINE **mm) {
    char pbuf[GUID_LEN + 1], mbuf[GUID_LEN + 1];

    if(!person_guid || !*person_guid || !machine_guid || !*machine_guid || !url || !*url) {
        info("Registry Request Verification: invalid request! person: '%s', machine '%s', url '%s'", person_guid?person_guid:"UNSET", machine_guid?machine_guid:"UNSET", url?url:"UNSET");
        return NULL;
    }

    // normalize the url
    url = registry_fix_url(url, NULL);

    // make sure the person GUID is valid
    if(regenerate_guid(person_guid, pbuf) == -1) {
        info("Registry Request Verification: invalid person GUID, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    person_guid = pbuf;

    // make sure the machine GUID is valid
    if(regenerate_guid(machine_guid, mbuf) == -1) {
        info("Registry Request Verification: invalid machine GUID, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    machine_guid = mbuf;

    // make sure the machine exists
    REGISTRY_MACHINE *m = registry_machine_find(machine_guid);
    if(!m) {
        info("Registry Request Verification: machine not found, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    if(mm) *mm = m;

    // make sure the person exist
    REGISTRY_PERSON *p = registry_person_find(person_guid);
    if(!p) {
        info("Registry Request Verification: person not found, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    if(pp) *pp = p;

    REGISTRY_PERSON_URL *pu = registry_person_url_index_find(p, url);
    if(!pu) {
        info("Registry Request Verification: URL not found for person, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    return pu;
}


// ----------------------------------------------------------------------------
// REGISTRY REQUESTS

REGISTRY_PERSON *registry_request_access(char *person_guid, char *machine_guid, char *url, char *name, time_t when) {
    debug(D_REGISTRY, "registry_request_access('%s', '%s', '%s'): NEW REQUEST", (person_guid)?person_guid:"", machine_guid, url);

    REGISTRY_MACHINE *m = registry_machine_get(machine_guid, when);
    if(!m) return NULL;

    // make sure the name is valid
    size_t namelen;
    name = registry_fix_machine_name(name, &namelen);

    size_t urllen;
    url = registry_fix_url(url, &urllen);

    REGISTRY_PERSON *p = registry_person_get(person_guid, when);

    REGISTRY_URL *u = registry_url_get(url, urllen);
    registry_person_link_to_url(p, m, u, name, namelen, when);
    registry_machine_link_to_url(m, u, when);

    registry_log('A', p, m, u, name);

    registry.usages_count++;

    return p;
}

REGISTRY_PERSON *registry_request_delete(char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when) {
    (void) when;

    REGISTRY_PERSON *p = NULL;
    REGISTRY_MACHINE *m = NULL;
    REGISTRY_PERSON_URL *pu = registry_verify_request(person_guid, machine_guid, url, &p, &m);
    if(!pu || !p || !m) return NULL;

    // normalize the url
    delete_url = registry_fix_url(delete_url, NULL);

    // make sure the user is not deleting the url it uses
    if(!strcmp(delete_url, pu->url->url)) {
        info("Registry Delete Request: delete URL is the one currently accessed, person: '%s', machine '%s', url '%s', delete url '%s'"
             , p->guid, m->guid, pu->url->url, delete_url);
        return NULL;
    }

    REGISTRY_PERSON_URL *dpu = registry_person_url_index_find(p, delete_url);
    if(!dpu) {
        info("Registry Delete Request: URL not found for person: '%s', machine '%s', url '%s', delete url '%s'", p->guid
             , m->guid, pu->url->url, delete_url);
        return NULL;
    }

    registry_log('D', p, m, pu->url, dpu->url->url);
    registry_person_unlink_from_url(p, dpu);

    return p;
}


// a structure to pass to the dictionary_get_all() callback handler
struct machine_request_callback_data {
    REGISTRY_MACHINE *find_this_machine;
    REGISTRY_PERSON_URL *result;
};

// the callback function
// this will be run for every PERSON_URL of this PERSON
static int machine_request_callback(void *entry, void *data) {
    REGISTRY_PERSON_URL *mypu = (REGISTRY_PERSON_URL *)entry;
    struct machine_request_callback_data *myrdata = (struct machine_request_callback_data *)data;

    if(mypu->machine == myrdata->find_this_machine) {
        myrdata->result = mypu;
        return -1; // this will also stop the walk through
    }

    return 0; // continue
}

REGISTRY_MACHINE *registry_request_machine(char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when) {
    (void)when;

    char mbuf[GUID_LEN + 1];

    REGISTRY_PERSON *p = NULL;
    REGISTRY_MACHINE *m = NULL;
    REGISTRY_PERSON_URL *pu = registry_verify_request(person_guid, machine_guid, url, &p, &m);
    if(!pu || !p || !m) return NULL;

    // make sure the machine GUID is valid
    if(regenerate_guid(request_machine, mbuf) == -1) {
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
    avl_traverse(&p->person_urls, machine_request_callback, &rdata);

    if(rdata.result)
        return m;

    return NULL;
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

char *registry_get_this_machine_hostname(void) {
    return registry.hostname;
}

char *registry_get_this_machine_guid(void) {
    static char guid[GUID_LEN + 1] = "";

    if(likely(guid[0]))
        return guid;

    // read it from disk
    int fd = open(registry.machine_guid_filename, O_RDONLY);
    if(fd != -1) {
        char buf[GUID_LEN + 1];
        if(read(fd, buf, GUID_LEN) != GUID_LEN)
            error("Failed to read machine GUID from '%s'", registry.machine_guid_filename);
        else {
            buf[GUID_LEN] = '\0';
            if(regenerate_guid(buf, guid) == -1) {
                error("Failed to validate machine GUID '%s' from '%s'. Ignoring it - this might mean this netdata will appear as duplicate in the registry.",
                        buf, registry.machine_guid_filename);

                guid[0] = '\0';
            }
            else if(is_machine_guid_blacklisted(guid))
                guid[0] = '\0';
        }
        close(fd);
    }

    // generate a new one?
    if(!guid[0]) {
        uuid_t uuid;

        uuid_generate_time(uuid);
        uuid_unparse_lower(uuid, guid);
        guid[GUID_LEN] = '\0';

        // save it
        fd = open(registry.machine_guid_filename, O_WRONLY|O_CREAT|O_TRUNC, 444);
        if(fd == -1)
            fatal("Cannot create unique machine id file '%s'. Please fix this.", registry.machine_guid_filename);

        if(write(fd, guid, GUID_LEN) != GUID_LEN)
            fatal("Cannot write the unique machine id file '%s'. Please fix this.", registry.machine_guid_filename);

        close(fd);
    }

    setenv("NETDATA_REGISTRY_UNIQUE_ID", guid, 1);

    return guid;
}
