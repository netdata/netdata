// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "registry_internals.h"

struct registry registry;

// ----------------------------------------------------------------------------
// common functions

// parse a GUID and re-generated to be always lower case
// this is used as a protection against the variations of GUIDs
int regenerate_guid(const char *guid, char *result) {
    nd_uuid_t uuid;
    if(unlikely(uuid_parse(guid, uuid) == -1)) {
        netdata_log_info("Registry: GUID '%s' is not a valid GUID.", guid);
        return -1;
    }
    else {
        uuid_unparse_lower(uuid, result);

#ifdef NETDATA_INTERNAL_CHECKS
        if(strcmp(guid, result) != 0)
            netdata_log_info("GUID '%s' and re-generated GUID '%s' differ!", guid, result);
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
    while(*s && isspace((uint8_t)*s)) s++;

    // make sure all spaces are a SPACE
    char *t = s;
    while(*t) {
        if(unlikely(isspace((uint8_t)*t)))
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
REGISTRY_PERSON_URL *registry_verify_request(const char *person_guid, char *machine_guid, char *url, REGISTRY_PERSON **pp, REGISTRY_MACHINE **mm) {
    char pbuf[GUID_LEN + 1], mbuf[GUID_LEN + 1];

    if(!person_guid || !*person_guid || !machine_guid || !*machine_guid || !url || !*url) {
        netdata_log_info("Registry Request Verification: invalid request! person: '%s', machine '%s', url '%s'", person_guid?person_guid:"UNSET", machine_guid?machine_guid:"UNSET", url?url:"UNSET");
        return NULL;
    }

    // normalize the url
    url = registry_fix_url(url, NULL);

    // make sure the person GUID is valid
    if(regenerate_guid(person_guid, pbuf) == -1) {
        netdata_log_info("Registry Request Verification: invalid person GUID, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    person_guid = pbuf;

    // make sure the machine GUID is valid
    if(regenerate_guid(machine_guid, mbuf) == -1) {
        netdata_log_info("Registry Request Verification: invalid machine GUID, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    machine_guid = mbuf;

    // make sure the machine exists
    REGISTRY_MACHINE *m = registry_machine_find(machine_guid);
    if(!m) {
        netdata_log_info("Registry Request Verification: machine not found, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    if(mm) *mm = m;

    // make sure the person exist
    REGISTRY_PERSON *p = registry_person_find(person_guid);
    if(!p) {
        netdata_log_info("Registry Request Verification: person not found, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    if(pp) *pp = p;

    STRING *u = string_strdupz(url);
    REGISTRY_PERSON_URL *pu = registry_person_url_index_find(p, u);
    string_freez(u);

    if(!pu) {
        netdata_log_info("Registry Request Verification: URL not found for person, person: '%s', machine '%s', url '%s'", person_guid, machine_guid, url);
        return NULL;
    }
    //else if (pu->machine != m) {
    //    netdata_log_info("Registry Request Verification: Machine mismatch: person: '%s', machine requested='%s' <> loaded='%s', url '%s'", person_guid, machine_guid, pu->machine->guid, url);
    //    return NULL;
    //}

    return pu;
}


// ----------------------------------------------------------------------------
// REGISTRY REQUESTS

REGISTRY_PERSON *registry_request_access(const char *person_guid, char *machine_guid, char *url, char *name, time_t when) {
    netdata_log_debug(D_REGISTRY, "registry_request_access('%s', '%s', '%s'): NEW REQUEST", (person_guid)?person_guid:"", machine_guid, url);

    bool is_dummy = is_dummy_person(person_guid);

    REGISTRY_MACHINE *m = registry_machine_find_or_create(machine_guid, when, is_dummy);
    if(!m) return NULL;

    REGISTRY_PERSON *p = registry_person_find_or_create(person_guid, when, is_dummy);

    // make sure the name is valid
    size_t name_len;
    name = registry_fix_machine_name(name, &name_len);

    size_t url_len;
    url = registry_fix_url(url, &url_len);

    STRING *u = string_strdupz(url);

    if(!is_dummy)
        registry_person_link_to_url(p, m, u, name, name_len, when);

    registry_machine_link_to_url(m, u, when);

    registry_log('A', p, m, u, name);

    registry.usages_count++;

    return p;
}

REGISTRY_PERSON *registry_request_delete(const char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when) {
    (void) when;

    REGISTRY_PERSON *p = NULL;
    REGISTRY_MACHINE *m = NULL;
    REGISTRY_PERSON_URL *pu = registry_verify_request(person_guid, machine_guid, url, &p, &m);
    if(!pu || !p || !m) return NULL;

    // normalize the url
    delete_url = registry_fix_url(delete_url, NULL);

    // make sure the user is not deleting the url it uses
    /*
    if(!strcmp(delete_url, pu->url->url)) {
        netdata_log_info("Registry Delete Request: delete URL is the one currently accessed, person: '%s', machine '%s', url '%s', delete url '%s'"
             , p->guid, m->guid, pu->url->url, delete_url);
        return NULL;
    }
    */

    STRING *d_url = string_strdupz(delete_url);
    REGISTRY_PERSON_URL *dpu = registry_person_url_index_find(p, d_url);
    string_freez(d_url);

    if(!dpu) {
        netdata_log_info("Registry Delete Request: URL not found for person: '%s', machine '%s', url '%s', delete url '%s'", p->guid
             , m->guid, string2str(pu->url), delete_url);
        return NULL;
    }

    registry_log('D', p, m, pu->url, string2str(dpu->url));
    registry_person_unlink_from_url(p, dpu);

    return p;
}


REGISTRY_MACHINE *registry_request_machine(const char *person_guid, char *request_machine, STRING **hostname) {
    char pbuf[GUID_LEN + 1];
    char mbuf[GUID_LEN + 1];

    // make sure the person GUID is valid
    if(regenerate_guid(person_guid, pbuf) == -1) {
        netdata_log_info("REGISTRY: %s(): invalid person GUID '%s'", __FUNCTION__ , person_guid);
        return NULL;
    }
    person_guid = pbuf;

    // make sure the person GUID is valid
    if(regenerate_guid(request_machine, mbuf) == -1) {
        netdata_log_info("REGISTRY: %s(): invalid search machine GUID '%s'", __FUNCTION__ , request_machine);
        return NULL;
    }
    request_machine = mbuf;

    REGISTRY_PERSON *p = registry_person_find(person_guid);
    if(!p) return NULL;

    REGISTRY_MACHINE *m = registry_machine_find(request_machine);
    if(!m) return NULL;

    // Verify the user has in the past accessed this machine
    // We will walk through the PERSON_URLs to find the machine
    // linking to our machine

    // make sure the user has access
    for(REGISTRY_PERSON_URL *pu = p->person_urls; pu ;pu = pu->next)
        if(pu->machine == m) {
            *hostname = string_dup(pu->machine_name);
            return m;
        }

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
        netdata_log_error("Blacklisted machine GUID '%s' found.", guid);
        return 1;
    }

    return 0;
}

const char *registry_get_this_machine_hostname(void) {
    return registry.hostname;
}

const char *registry_get_this_machine_guid(bool create_it) {
    static char guid[GUID_LEN + 1] = "";

    if(likely(guid[0]))
        return guid;

    // read it from disk
    int fd = open(registry.machine_guid_filename, O_RDONLY | O_CLOEXEC);
    if(fd != -1) {
        char buf[GUID_LEN + 1];
        if(read(fd, buf, GUID_LEN) != GUID_LEN)
            netdata_log_error("Failed to read machine GUID from '%s'", registry.machine_guid_filename);
        else {
            buf[GUID_LEN] = '\0';
            if(regenerate_guid(buf, guid) == -1) {
                netdata_log_error("Failed to validate machine GUID '%s' from '%s'. Ignoring it - this might mean this netdata will appear as duplicate in the registry.",
                        buf, registry.machine_guid_filename);

                guid[0] = '\0';
            }
            else if(is_machine_guid_blacklisted(guid))
                guid[0] = '\0';
        }
        close(fd);
    }

    // generate a new one?
    if(!guid[0] && create_it) {
        nd_uuid_t uuid;

        uuid_generate_time(uuid);
        uuid_unparse_lower(uuid, guid);
        guid[GUID_LEN] = '\0';

        // save it
        fd = open(registry.machine_guid_filename, O_WRONLY|O_CREAT|O_TRUNC | O_CLOEXEC, 444);
        if(fd == -1)
            fatal("Cannot create unique machine id file '%s'. Please fix this.", registry.machine_guid_filename);

        if(write(fd, guid, GUID_LEN) != GUID_LEN)
            fatal("Cannot write the unique machine id file '%s'. Please fix this.", registry.machine_guid_filename);

        close(fd);
    }

    if(guid[0])
        nd_setenv("NETDATA_REGISTRY_UNIQUE_ID", guid, 1);

    return guid;
}
