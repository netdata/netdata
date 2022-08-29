// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"

// ----------------------------------------------------------------------------
// REGISTRY concurrency locking

static inline void registry_lock(void) {
    netdata_mutex_lock(&registry.lock);
}

static inline void registry_unlock(void) {
    netdata_mutex_unlock(&registry.lock);
}


// ----------------------------------------------------------------------------
// COOKIES

static void registry_set_cookie(struct web_client *w, const char *guid) {
    char edate[100], domain[512];
    time_t et = now_realtime_sec() + registry.persons_expiration;
    struct tm etmbuf, *etm = gmtime_r(&et, &etmbuf);
    strftime(edate, sizeof(edate), "%a, %d %b %Y %H:%M:%S %Z", etm);

    snprintfz(w->cookie1, NETDATA_WEB_REQUEST_COOKIE_SIZE, NETDATA_REGISTRY_COOKIE_NAME "=%s; Expires=%s", guid, edate);

    if(registry.registry_domain && registry.registry_domain[0])
        snprintfz(domain, 511, "Domain=%s", registry.registry_domain);
    else
        domain[0]='\0';

    int length = snprintfz(w->cookie2, NETDATA_WEB_REQUEST_COOKIE_SIZE,
                           NETDATA_REGISTRY_COOKIE_NAME "=%s; Expires=%s; %s",
                           guid, edate, domain);

    size_t remaining_length = NETDATA_WEB_REQUEST_COOKIE_SIZE - length;
    // 25 is the necessary length to add new cookies
    if (registry.enable_cookies_samesite_secure) {
        if (length > 0 && remaining_length > 25)
            snprintfz(&w->cookie2[length], remaining_length, "; SameSite=None; Secure");
        else
            error("Netdata does not have enough space to store cookies SameSite and Secure");
    }
}

static inline void registry_set_person_cookie(struct web_client *w, REGISTRY_PERSON *p) {
    registry_set_cookie(w, p->guid);
}


// ----------------------------------------------------------------------------
// JSON GENERATION

static inline void registry_json_header(RRDHOST *host, struct web_client *w, const char *action, const char *status) {
    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    buffer_sprintf(w->response.data, "{\n\t\"action\": \"%s\",\n\t\"status\": \"%s\",\n\t\"hostname\": \"%s\",\n\t\"machine_guid\": \"%s\"",
            action, status, rrdhost_registry_hostname(host), host->machine_guid);
}

static inline void registry_json_footer(struct web_client *w) {
    buffer_strcat(w->response.data, "\n}\n");
}

static inline int registry_json_disabled(RRDHOST *host, struct web_client *w, const char *action) {
    registry_json_header(host, w, action, REGISTRY_STATUS_DISABLED);

    buffer_sprintf(w->response.data, ",\n\t\"registry\": \"%s\"",
            registry.registry_to_announce);

    registry_json_footer(w);
    return 200;
}


// ----------------------------------------------------------------------------
// CALLBACKS FOR WALKING THROUGH REGISTRY OBJECTS

// structure used be the callbacks below
struct registry_json_walk_person_urls_callback {
    REGISTRY_PERSON *p;
    REGISTRY_MACHINE *m;
    struct web_client *w;
    int count;
};

// callback for rendering PERSON_URLs
static int registry_json_person_url_callback(void *entry, void *data) {
    REGISTRY_PERSON_URL *pu = (REGISTRY_PERSON_URL *)entry;
    struct registry_json_walk_person_urls_callback *c = (struct registry_json_walk_person_urls_callback *)data;
    struct web_client *w = c->w;

    if (!strcmp(pu->url->url,"***")) return 0;
    
    if(unlikely(c->count++))
        buffer_strcat(w->response.data, ",");

    buffer_sprintf(w->response.data, "\n\t\t[ \"%s\", \"%s\", %u000, %u, \"%s\" ]",
            pu->machine->guid, pu->url->url, pu->last_t, pu->usages, pu->machine_name);

    return 0;
}

// callback for rendering MACHINE_URLs
static int registry_json_machine_url_callback(const char *name, void *entry, void *data) {
    (void)name;

    REGISTRY_MACHINE_URL *mu = (REGISTRY_MACHINE_URL *)entry;
    struct registry_json_walk_person_urls_callback *c = (struct registry_json_walk_person_urls_callback *)data;
    struct web_client *w = c->w;
    REGISTRY_MACHINE *m = c->m;

    if (!strcmp(mu->url->url,"***")) return 1;

    if(unlikely(c->count++))
        buffer_strcat(w->response.data, ",");

    buffer_sprintf(w->response.data, "\n\t\t[ \"%s\", \"%s\", %u000, %u ]",
            m->guid, mu->url->url, mu->last_t, mu->usages);

    return 1;
}

// ----------------------------------------------------------------------------

// structure used be the callbacks below
struct registry_person_url_callback_verify_machine_exists_data {
    REGISTRY_MACHINE *m;
    int count;
};

static inline int registry_person_url_callback_verify_machine_exists(void *entry, void *data) {
    struct registry_person_url_callback_verify_machine_exists_data *d = (struct registry_person_url_callback_verify_machine_exists_data *)data;
    REGISTRY_PERSON_URL *pu = (REGISTRY_PERSON_URL *)entry;
    REGISTRY_MACHINE *m = d->m;

    if(pu->machine == m)
        d->count++;

    return 0;
}

// ----------------------------------------------------------------------------
// dynamic update of the configuration
// The registry does not seem to be designed to support this and I cannot see any concurrency protection
// that could make this safe, so try to be as atomic as possible.

void registry_update_cloud_base_url()
{
    // This is guaranteed to be set early in main via post_conf_load()
    registry.cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
    if (registry.cloud_base_url == NULL)
        fatal("Do not move the cloud base url out of post_conf_load!!");

    setenv("NETDATA_REGISTRY_CLOUD_BASE_URL", registry.cloud_base_url, 1);
}
// ----------------------------------------------------------------------------
// public HELLO request

int registry_request_hello_json(RRDHOST *host, struct web_client *w) {
    registry_json_header(host, w, "hello", REGISTRY_STATUS_OK);

    buffer_sprintf(w->response.data,
            ",\n\t\"registry\": \"%s\",\n\t\"cloud_base_url\": \"%s\",\n\t\"anonymous_statistics\": %s",
            registry.registry_to_announce,
            registry.cloud_base_url, netdata_anonymous_statistics_enabled?"true":"false");

    registry_json_footer(w);
    return 200;
}

// ----------------------------------------------------------------------------
//public ACCESS request

#define REGISTRY_VERIFY_COOKIES_GUID "give-me-back-this-cookie-now--please"

// the main method for registering an access
int registry_request_access_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *name, time_t when) {
    if(unlikely(!registry.enabled))
        return registry_json_disabled(host, w, "access");

    // ------------------------------------------------------------------------
    // verify the browser supports cookies

    if(registry.verify_cookies_redirects > 0 && !person_guid[0]) {
        buffer_flush(w->response.data);
        registry_set_cookie(w, REGISTRY_VERIFY_COOKIES_GUID);
        w->response.data->contenttype = CT_APPLICATION_JSON;
        buffer_sprintf(w->response.data, "{ \"status\": \"redirect\", \"registry\": \"%s\" }", registry.registry_to_announce);
        return 200;
    }

    if(unlikely(person_guid[0] && !strcmp(person_guid, REGISTRY_VERIFY_COOKIES_GUID)))
        person_guid[0] = '\0';

    // ------------------------------------------------------------------------

    registry_lock();

    REGISTRY_PERSON *p = registry_request_access(person_guid, machine_guid, url, name, when);
    if(!p) {
        registry_json_header(host, w, "access", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 412;
    }

    // set the cookie
    registry_set_person_cookie(w, p);

    // generate the response
    registry_json_header(host, w, "access", REGISTRY_STATUS_OK);

    buffer_sprintf(w->response.data, ",\n\t\"person_guid\": \"%s\",\n\t\"urls\": [", p->guid);
    struct registry_json_walk_person_urls_callback c = { p, NULL, w, 0 };
    avl_traverse(&p->person_urls, registry_json_person_url_callback, &c);
    buffer_strcat(w->response.data, "\n\t]\n");

    registry_json_footer(w);
    registry_unlock();
    return 200;
}

// ----------------------------------------------------------------------------
// public DELETE request

// the main method for deleting a URL from a person
int registry_request_delete_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when) {
    if(!registry.enabled)
        return registry_json_disabled(host, w, "delete");

    registry_lock();

    REGISTRY_PERSON *p = registry_request_delete(person_guid, machine_guid, url, delete_url, when);
    if(!p) {
        registry_json_header(host, w, "delete", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 412;
    }

    // generate the response
    registry_json_header(host, w, "delete", REGISTRY_STATUS_OK);
    registry_json_footer(w);
    registry_unlock();
    return 200;
}

// ----------------------------------------------------------------------------
// public SEARCH request

// the main method for searching the URLs of a netdata
int registry_request_search_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when) {
    if(!registry.enabled)
        return registry_json_disabled(host, w, "search");

    registry_lock();

    REGISTRY_MACHINE *m = registry_request_machine(person_guid, machine_guid, url, request_machine, when);
    if(!m) {
        registry_json_header(host, w, "search", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 404;
    }

    registry_json_header(host, w, "search", REGISTRY_STATUS_OK);

    buffer_strcat(w->response.data, ",\n\t\"urls\": [");
    struct registry_json_walk_person_urls_callback c = { NULL, m, w, 0 };
    dictionary_walkthrough_read(m->machine_urls, registry_json_machine_url_callback, &c);
    buffer_strcat(w->response.data, "\n\t]\n");

    registry_json_footer(w);
    registry_unlock();
    return 200;
}

// ----------------------------------------------------------------------------
// SWITCH REQUEST

// the main method for switching user identity
int registry_request_switch_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *new_person_guid, time_t when) {
    if(!registry.enabled)
        return registry_json_disabled(host, w, "switch");

    (void)url;
    (void)when;

    registry_lock();

    REGISTRY_PERSON *op = registry_person_find(person_guid);
    if(!op) {
        registry_json_header(host, w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 430;
    }

    REGISTRY_PERSON *np = registry_person_find(new_person_guid);
    if(!np) {
        registry_json_header(host, w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 431;
    }

    REGISTRY_MACHINE *m = registry_machine_find(machine_guid);
    if(!m) {
        registry_json_header(host, w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 432;
    }

    struct registry_person_url_callback_verify_machine_exists_data data = { m, 0 };

    // verify the old person has access to this machine
    avl_traverse(&op->person_urls, registry_person_url_callback_verify_machine_exists, &data);
    if(!data.count) {
        registry_json_header(host, w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 433;
    }

    // verify the new person has access to this machine
    data.count = 0;
    avl_traverse(&np->person_urls, registry_person_url_callback_verify_machine_exists, &data);
    if(!data.count) {
        registry_json_header(host, w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 434;
    }

    // set the cookie of the new person
    // the user just switched identity
    registry_set_person_cookie(w, np);

    // generate the response
    registry_json_header(host, w, "switch", REGISTRY_STATUS_OK);
    buffer_sprintf(w->response.data, ",\n\t\"person_guid\": \"%s\"", np->guid);
    registry_json_footer(w);

    registry_unlock();
    return 200;
}

// ----------------------------------------------------------------------------
// STATISTICS

void registry_statistics(void) {
    if(!registry.enabled) return;

    static RRDSET *sts = NULL, *stc = NULL, *stm = NULL;

    if(unlikely(!sts)) {
        sts = rrdset_create_localhost(
                "netdata"
                , "registry_sessions"
                , NULL
                , "registry"
                , NULL
                , "Netdata Registry Sessions"
                , "sessions"
                , "registry"
                , "stats"
                , 131000
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
        );

        rrddim_add(sts, "sessions",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    else rrdset_next(sts);

    rrddim_set(sts, "sessions", registry.usages_count);
    rrdset_done(sts);

    // ------------------------------------------------------------------------

    if(unlikely(!stc)) {
        stc = rrdset_create_localhost(
                "netdata"
                , "registry_entries"
                , NULL
                , "registry"
                , NULL
                , "Netdata Registry Entries"
                , "entries"
                , "registry"
                , "stats"
                , 131100
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
        );

        rrddim_add(stc, "persons",        NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stc, "machines",       NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stc, "urls",           NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stc, "persons_urls",   NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stc, "machines_urls",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    else rrdset_next(stc);

    rrddim_set(stc, "persons",       registry.persons_count);
    rrddim_set(stc, "machines",      registry.machines_count);
    rrddim_set(stc, "urls",          registry.urls_count);
    rrddim_set(stc, "persons_urls",  registry.persons_urls_count);
    rrddim_set(stc, "machines_urls", registry.machines_urls_count);
    rrdset_done(stc);

    // ------------------------------------------------------------------------

    if(unlikely(!stm)) {
        stm = rrdset_create_localhost(
                "netdata"
                , "registry_mem"
                , NULL
                , "registry"
                , NULL
                , "Netdata Registry Memory"
                , "KiB"
                , "registry"
                , "stats"
                , 131300
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
        );

        rrddim_add(stm, "persons",        NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stm, "machines",       NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stm, "urls",           NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stm, "persons_urls",   NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stm, "machines_urls",  NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
    }
    else rrdset_next(stm);

    rrddim_set(stm, "persons",       registry.persons_memory + dictionary_stats_allocated_memory(registry.persons));
    rrddim_set(stm, "machines",      registry.machines_memory + dictionary_stats_allocated_memory(registry.machines));
    rrddim_set(stm, "urls",          registry.urls_memory);
    rrddim_set(stm, "persons_urls",  registry.persons_urls_memory);
    rrddim_set(stm, "machines_urls", registry.machines_urls_memory);
    rrdset_done(stm);
}
