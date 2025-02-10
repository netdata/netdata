// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "registry_internals.h"

#define REGISTRY_STATUS_OK "ok"
#define REGISTRY_STATUS_REDIRECT "redirect"
#define REGISTRY_STATUS_FAILED "failed"
#define REGISTRY_STATUS_DISABLED "disabled"

bool registry_is_valid_url(const char *url) {
    return url && (*url == 'h' || *url == '*');
}

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
    char rfc7231_expires[RFC7231_MAX_LENGTH];
    rfc7231_datetime(rfc7231_expires, sizeof(rfc7231_expires), now_realtime_sec() + registry.persons_expiration);

    buffer_sprintf(w->response.header, "Set-Cookie: " NETDATA_REGISTRY_COOKIE_NAME "=%s; Expires=%s\r\n", guid, rfc7231_expires);
    buffer_sprintf(w->response.header, "Set-Cookie: " NETDATA_REGISTRY_COOKIE_NAME "=%s; SameSite=Strict; Expires=%s\r\n", guid, rfc7231_expires);
    if(registry.enable_cookies_samesite_secure)
        buffer_sprintf(w->response.header, "Set-Cookie: " NETDATA_REGISTRY_COOKIE_NAME "=%s; Expires=%s; SameSite=None; Secure\r\n", guid, rfc7231_expires);

    if(registry.registry_domain && *registry.registry_domain) {
        buffer_sprintf(w->response.header, "Set-Cookie: " NETDATA_REGISTRY_COOKIE_NAME "=%s; Expires=%s; Domain=%s\r\n", guid, rfc7231_expires, registry.registry_domain);
        buffer_sprintf(w->response.header, "Set-Cookie: " NETDATA_REGISTRY_COOKIE_NAME "=%s; Expires=%s; Domain=%s; SameSite=Strict\r\n", guid, rfc7231_expires, registry.registry_domain);
        if(registry.enable_cookies_samesite_secure)
            buffer_sprintf(w->response.header, "Set-Cookie: " NETDATA_REGISTRY_COOKIE_NAME "=%s; Expires=%s; Domain=%s; SameSite=None; Secure\r\n", guid, rfc7231_expires, registry.registry_domain);
    }

    w->response.has_cookies = true;
}

static inline void registry_set_person_cookie(struct web_client *w, REGISTRY_PERSON *p) {
    registry_set_cookie(w, p->guid);
}


// ----------------------------------------------------------------------------
// JSON GENERATION

static inline void registry_json_header(RRDHOST *host, struct web_client *w, const char *action, const char *status) {
    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(w->response.data, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(w->response.data, "action", action);
    buffer_json_member_add_string(w->response.data, "status", status);
    buffer_json_member_add_string(w->response.data, "hostname", rrdhost_registry_hostname(host));
    buffer_json_member_add_string(w->response.data, "machine_guid", host->machine_guid);
}

static inline void registry_json_footer(struct web_client *w) {
    buffer_json_finalize(w->response.data);
}

static inline int registry_json_disabled(RRDHOST *host, struct web_client *w, const char *action) {
    registry_json_header(host, w, action, REGISTRY_STATUS_DISABLED);

    buffer_json_member_add_string(w->response.data, "registry", registry.registry_to_announce);

    registry_json_footer(w);
    return HTTP_RESP_OK;
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

static STRING *asterisks = NULL;

// callback for rendering PERSON_URLs
static int registry_json_person_url_callback(REGISTRY_PERSON_URL *pu, struct registry_json_walk_person_urls_callback *c) {
    if(unlikely(!asterisks))
        asterisks = string_strdupz("***");

    struct web_client *w = c->w;

    if (pu->url == asterisks) return 0;

    buffer_json_add_array_item_array(w->response.data);
    buffer_json_add_array_item_string(w->response.data, pu->machine->guid);
    buffer_json_add_array_item_string(w->response.data, string2str(pu->url));
    buffer_json_add_array_item_uint64(w->response.data, pu->last_t * (uint64_t) 1000);
    buffer_json_add_array_item_uint64(w->response.data, pu->usages);
    buffer_json_add_array_item_string(w->response.data, string2str(pu->machine_name));
    buffer_json_array_close(w->response.data);

    return 1;
}

// callback for rendering MACHINE_URLs
static int registry_json_machine_url_callback(REGISTRY_MACHINE_URL *mu, struct registry_json_walk_person_urls_callback *c, STRING *hostname) {
    if(unlikely(!asterisks))
        asterisks = string_strdupz("***");

    struct web_client *w = c->w;
    REGISTRY_MACHINE *m = c->m;

    if (mu->url == asterisks) return 0;

    buffer_json_add_array_item_array(w->response.data);
    buffer_json_add_array_item_string(w->response.data, m->guid);
    buffer_json_add_array_item_string(w->response.data, string2str(mu->url));
    buffer_json_add_array_item_uint64(w->response.data, mu->last_t * (uint64_t) 1000);
    buffer_json_add_array_item_uint64(w->response.data, mu->usages);
    buffer_json_add_array_item_string(w->response.data, string2str(hostname));
    buffer_json_array_close(w->response.data);

    return 1;
}

// ----------------------------------------------------------------------------

// structure used be the callbacks below
struct registry_person_url_callback_verify_machine_exists_data {
    REGISTRY_MACHINE *m;
    int count;
};

static inline int registry_person_url_callback_verify_machine_exists(REGISTRY_PERSON_URL *pu, struct registry_person_url_callback_verify_machine_exists_data *d) {
    REGISTRY_MACHINE *m = d->m;

    if(pu->machine == m)
        d->count++;

    return 0;
}

// ----------------------------------------------------------------------------
// dynamic update of the configuration
// The registry does not seem to be designed to support this and I cannot see any concurrency protection
// that could make this safe, so try to be as atomic as possible.

void registry_update_cloud_base_url() {
    registry.cloud_base_url = cloud_config_url_get();
    nd_setenv("NETDATA_REGISTRY_CLOUD_BASE_URL", registry.cloud_base_url, 1);
}

// ----------------------------------------------------------------------------
// public HELLO request

int registry_request_hello_json(RRDHOST *host, struct web_client *w, bool do_not_track) {
    registry_json_header(host, w, "hello", REGISTRY_STATUS_OK);

    if(!UUIDiszero(host->node_id))
        buffer_json_member_add_uuid(w->response.data, "node_id", host->node_id.uuid);

    buffer_json_member_add_object(w->response.data, "agent");
    {
        buffer_json_member_add_string(w->response.data, "machine_guid", localhost->machine_guid);

        if(!UUIDiszero(localhost->node_id))
            buffer_json_member_add_uuid(w->response.data, "node_id", localhost->node_id.uuid);

        CLAIM_ID claim_id = rrdhost_claim_id_get(host);
        if (claim_id_is_set(claim_id))
            buffer_json_member_add_string(w->response.data, "claim_id", claim_id.str);

        buffer_json_member_add_boolean(w->response.data, "bearer_protection", netdata_is_protected_by_bearer);
    }
    buffer_json_object_close(w->response.data);

    CLOUD_STATUS status = cloud_status();
    buffer_json_member_add_string(w->response.data, "cloud_status", cloud_status_to_string(status));
    buffer_json_member_add_string(w->response.data, "cloud_base_url", registry.cloud_base_url);

    buffer_json_member_add_string(w->response.data, "registry", registry.registry_to_announce);
    buffer_json_member_add_boolean(w->response.data, "anonymous_statistics", do_not_track ? false : netdata_anonymous_statistics_enabled);
    buffer_json_member_add_boolean(w->response.data, "X-Netdata-Auth", true);

    buffer_json_member_add_array(w->response.data, "nodes");
    RRDHOST *h;
    dfe_start_read(rrdhost_root_index, h) {
        buffer_json_add_array_item_object(w->response.data);
        buffer_json_member_add_string(w->response.data, "machine_guid", h->machine_guid);

        if(!UUIDiszero(h->node_id))
            buffer_json_member_add_uuid(w->response.data, "node_id", h->node_id.uuid);

        buffer_json_member_add_string(w->response.data, "hostname", rrdhost_registry_hostname(h));
        buffer_json_object_close(w->response.data);
    }
    dfe_done(h);
    buffer_json_array_close(w->response.data); // nodes

    registry_json_footer(w);
    return HTTP_RESP_OK;
}

// ----------------------------------------------------------------------------
// public ACCESS request

// the main method for registering an access
int registry_request_access_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *name, time_t when) {
    if(unlikely(!registry.enabled))
        return registry_json_disabled(host, w, "access");

    if(!registry_is_valid_url(url)) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Invalid URL given in the request");
        return HTTP_RESP_BAD_REQUEST;
    }

    // ------------------------------------------------------------------------
    // verify the browser supports cookies or the bearer

    if(registry.verify_cookies_redirects > 0 && !person_guid[0]) {
        registry_lock();
        registry_request_access(REGISTRY_VERIFY_COOKIES_GUID, machine_guid, url, name, when);
        registry_unlock();

        buffer_flush(w->response.data);
        registry_set_cookie(w, REGISTRY_VERIFY_COOKIES_GUID);
        w->response.data->content_type = CT_APPLICATION_JSON;
        registry_json_header(host, w, "access", REGISTRY_STATUS_REDIRECT);
        buffer_json_member_add_string(w->response.data, "person_guid", REGISTRY_VERIFY_COOKIES_GUID);
        buffer_json_member_add_string(w->response.data, "registry", registry.registry_to_announce);
        registry_json_footer(w);
        return HTTP_RESP_OK;
    }

    if(unlikely(person_guid[0] && is_dummy_person(person_guid)))
        // it passed the check - they gave us a different person_guid
        // empty the dummy one, so that we will generate a new person_guid
        person_guid[0] = '\0';

    // ------------------------------------------------------------------------

    registry_lock();

    REGISTRY_PERSON *p = registry_request_access(person_guid, machine_guid, url, name, when);
    if(!p) {
        registry_json_header(host, w, "access", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    // set the cookie
    registry_set_person_cookie(w, p);

    // generate the response
    registry_json_header(host, w, "access", REGISTRY_STATUS_OK);
    buffer_json_member_add_string(w->response.data, "person_guid", p->guid);
    buffer_json_member_add_array(w->response.data, "urls");

    struct registry_json_walk_person_urls_callback c = { p, NULL, w, 0 };
    for(REGISTRY_PERSON_URL *pu = p->person_urls; pu ;pu = pu->next)
        registry_json_person_url_callback(pu, &c);
    buffer_json_array_close(w->response.data); // urls

    registry_json_footer(w);
    registry_unlock();
    return HTTP_RESP_OK;
}

// ----------------------------------------------------------------------------
// public DELETE request

// the main method for deleting a URL from a person
int registry_request_delete_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when) {
    if(!registry.enabled)
        return registry_json_disabled(host, w, "delete");

    if(!registry_is_valid_url(url)) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Invalid URL given in the request");
        return HTTP_RESP_BAD_REQUEST;
    }

    registry_lock();

    REGISTRY_PERSON *p = registry_request_delete(person_guid, machine_guid, url, delete_url, when);
    if(!p) {
        registry_json_header(host, w, "delete", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return HTTP_RESP_BAD_REQUEST;
    }

    // generate the response
    registry_json_header(host, w, "delete", REGISTRY_STATUS_OK);
    registry_json_footer(w);
    registry_unlock();
    return HTTP_RESP_OK;
}

// ----------------------------------------------------------------------------
// public SEARCH request

// the main method for searching the URLs of a netdata
int registry_request_search_json(RRDHOST *host, struct web_client *w, char *person_guid, char *request_machine) {
    if(!registry.enabled)
        return registry_json_disabled(host, w, "search");

    if(!person_guid || !person_guid[0]) {
        registry_json_header(host, w, "search", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        return HTTP_RESP_BAD_REQUEST;
    }

    registry_lock();

    STRING *hostname = NULL;
    REGISTRY_MACHINE *m = registry_request_machine(person_guid, request_machine, &hostname);
    if(!m) {
        registry_json_header(host, w, "search", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        string_freez(hostname);
        return HTTP_RESP_NOT_FOUND;
    }

    registry_json_header(host, w, "search", REGISTRY_STATUS_OK);

    buffer_json_member_add_array(w->response.data, "urls");
    struct registry_json_walk_person_urls_callback c = { NULL, m, w, 0 };

    for(REGISTRY_MACHINE_URL *mu = m->machine_urls; mu ; mu = mu->next)
        registry_json_machine_url_callback(mu, &c, hostname);

    buffer_json_array_close(w->response.data);

    registry_json_footer(w);
    registry_unlock();
    string_freez(hostname);
    return HTTP_RESP_OK;
}

// ----------------------------------------------------------------------------
// SWITCH REQUEST

// the main method for switching user identity
int registry_request_switch_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url __maybe_unused, char *new_person_guid, time_t when __maybe_unused) {
    if(!registry.enabled)
        return registry_json_disabled(host, w, "switch");

    if(!person_guid || !person_guid[0]) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Who are you? Person GUID is missing");
        return HTTP_RESP_BAD_REQUEST;
    }

    if(!registry_is_valid_url(url)) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Invalid URL given in the request");
        return HTTP_RESP_BAD_REQUEST;
    }

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
    for(REGISTRY_PERSON_URL *pu = op->person_urls; pu ;pu = pu->next)
        registry_person_url_callback_verify_machine_exists(pu, &data);

    if(!data.count) {
        registry_json_header(host, w, "switch", REGISTRY_STATUS_FAILED);
        registry_json_footer(w);
        registry_unlock();
        return 433;
    }

    // verify the new person has access to this machine
    data.count = 0;
    for(REGISTRY_PERSON_URL *pu = np->person_urls; pu ;pu = pu->next)
        registry_person_url_callback_verify_machine_exists(pu, &data);

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
    buffer_json_member_add_string(w->response.data, "person_guid", np->guid);
    registry_json_footer(w);

    registry_unlock();
    return HTTP_RESP_OK;
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

    rrddim_set(sts, "sessions", (collected_number)registry.usages_count);
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
        rrddim_add(stc, "persons_urls",   NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stc, "machines_urls",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set(stc, "persons",       (collected_number)registry.persons_count);
    rrddim_set(stc, "machines",      (collected_number)registry.machines_count);
    rrddim_set(stc, "persons_urls",  (collected_number)registry.persons_urls_count);
    rrddim_set(stc, "machines_urls", (collected_number)registry.machines_urls_count);
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
        rrddim_add(stm, "persons_urls",   NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stm, "machines_urls",  NULL,  1, 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    struct aral_statistics *p_aral_stats = aral_get_statistics(registry.persons_aral);
    rrddim_set(stm, "persons",       (collected_number)p_aral_stats->structures.allocated_bytes + (collected_number)p_aral_stats->malloc.allocated_bytes + (collected_number)p_aral_stats->mmap.allocated_bytes);

    struct aral_statistics *m_aral_stats = aral_get_statistics(registry.machines_aral);
    rrddim_set(stm, "machines",      (collected_number)m_aral_stats->structures.allocated_bytes + (collected_number)m_aral_stats->malloc.allocated_bytes + (collected_number)m_aral_stats->mmap.allocated_bytes);

    struct aral_statistics *pu_aral_stats = aral_get_statistics(registry.person_urls_aral);
    rrddim_set(stm, "persons_urls",  (collected_number)pu_aral_stats->structures.allocated_bytes + (collected_number)pu_aral_stats->malloc.allocated_bytes + (collected_number)pu_aral_stats->mmap.allocated_bytes);

    struct aral_statistics *mu_aral_stats = aral_get_statistics(registry.machine_urls_aral);
    rrddim_set(stm, "machines_urls", (collected_number)mu_aral_stats->structures.allocated_bytes + (collected_number)mu_aral_stats->malloc.allocated_bytes + (collected_number)mu_aral_stats->mmap.allocated_bytes);

    rrdset_done(stm);
}
