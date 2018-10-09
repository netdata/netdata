// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "registry_internals.h"

int registry_init(void) {
    char filename[FILENAME_MAX + 1];

    // registry enabled?
    if(web_server_mode != WEB_SERVER_MODE_NONE) {
        registry.enabled = config_get_boolean(CONFIG_SECTION_REGISTRY, "enabled", 0);
    }
    else {
        info("Registry is disabled - use the central netdata");
        config_set_boolean(CONFIG_SECTION_REGISTRY, "enabled", 0);
        registry.enabled = 0;
    }

    // pathnames
    snprintfz(filename, FILENAME_MAX, "%s/registry", netdata_configured_varlib_dir);
    registry.pathname = config_get(CONFIG_SECTION_REGISTRY, "registry db directory", filename);
    if(mkdir(registry.pathname, 0770) == -1 && errno != EEXIST)
        fatal("Cannot create directory '%s'.", registry.pathname);

    // filenames
    snprintfz(filename, FILENAME_MAX, "%s/netdata.public.unique.id", registry.pathname);
    registry.machine_guid_filename = config_get(CONFIG_SECTION_REGISTRY, "netdata unique id file", filename);

    snprintfz(filename, FILENAME_MAX, "%s/registry.db", registry.pathname);
    registry.db_filename = config_get(CONFIG_SECTION_REGISTRY, "registry db file", filename);

    snprintfz(filename, FILENAME_MAX, "%s/registry-log.db", registry.pathname);
    registry.log_filename = config_get(CONFIG_SECTION_REGISTRY, "registry log file", filename);

    // configuration options
    registry.save_registry_every_entries = (unsigned long long)config_get_number(CONFIG_SECTION_REGISTRY, "registry save db every new entries", 1000000);
    registry.persons_expiration = config_get_number(CONFIG_SECTION_REGISTRY, "registry expire idle persons days", 365) * 86400;
    registry.registry_domain = config_get(CONFIG_SECTION_REGISTRY, "registry domain", "");
    registry.registry_to_announce = config_get(CONFIG_SECTION_REGISTRY, "registry to announce", "https://registry.my-netdata.io");
    registry.hostname = config_get(CONFIG_SECTION_REGISTRY, "registry hostname", netdata_configured_hostname);
    registry.verify_cookies_redirects = config_get_boolean(CONFIG_SECTION_REGISTRY, "verify browser cookies support", 1);

    setenv("NETDATA_REGISTRY_HOSTNAME", registry.hostname, 1);
    setenv("NETDATA_REGISTRY_URL", registry.registry_to_announce, 1);

    registry.max_url_length = (size_t)config_get_number(CONFIG_SECTION_REGISTRY, "max URL length", 1024);
    if(registry.max_url_length < 10) {
        registry.max_url_length = 10;
        config_set_number(CONFIG_SECTION_REGISTRY, "max URL length", (long long)registry.max_url_length);
    }

    registry.max_name_length = (size_t)config_get_number(CONFIG_SECTION_REGISTRY, "max URL name length", 50);
    if(registry.max_name_length < 10) {
        registry.max_name_length = 10;
        config_set_number(CONFIG_SECTION_REGISTRY, "max URL name length", (long long)registry.max_name_length);
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
    netdata_mutex_init(&registry.lock);

    // create dictionaries
    registry.persons = dictionary_create(DICTIONARY_FLAGS);
    registry.machines = dictionary_create(DICTIONARY_FLAGS);
    avl_init(&registry.registry_urls_root_index, registry_url_compare);

    // load the registry database
    if(registry.enabled) {
        registry_log_open();
        registry_db_load();
        registry_log_load();

        if(unlikely(registry_db_should_be_saved()))
            registry_db_save();
    }

    return 0;
}

void registry_free(void) {
    if(!registry.enabled) return;

    // we need to destroy the dictionaries ourselves
    // since the dictionaries use memory we allocated

    while(registry.persons->values_index.root) {
        REGISTRY_PERSON *p = ((NAME_VALUE *)registry.persons->values_index.root)->value;
        registry_person_del(p);
    }

    while(registry.machines->values_index.root) {
        REGISTRY_MACHINE *m = ((NAME_VALUE *)registry.machines->values_index.root)->value;

        // fprintf(stderr, "\nMACHINE: '%s', first: %u, last: %u, usages: %u\n", m->guid, m->first_t, m->last_t, m->usages);

        while(m->machine_urls->values_index.root) {
            REGISTRY_MACHINE_URL *mu = ((NAME_VALUE *)m->machine_urls->values_index.root)->value;

            // fprintf(stderr, "\tURL: '%s', first: %u, last: %u, usages: %u, flags: 0x%02x\n", mu->url->url, mu->first_t, mu->last_t, mu->usages, mu->flags);

            //debug(D_REGISTRY, "Registry: destroying persons dictionary from url '%s'", mu->url->url);
            //dictionary_destroy(mu->persons);

            debug(D_REGISTRY, "Registry: deleting url '%s' from person '%s'", mu->url->url, m->guid);
            dictionary_del(m->machine_urls, mu->url->url);

            debug(D_REGISTRY, "Registry: unlinking url '%s' from machine", mu->url->url);
            registry_url_unlink(mu->url);

            debug(D_REGISTRY, "Registry: freeing machine url");
            freez(mu);
        }

        debug(D_REGISTRY, "Registry: deleting machine '%s' from machines registry", m->guid);
        dictionary_del(registry.machines, m->guid);

        debug(D_REGISTRY, "Registry: destroying URL dictionary of machine '%s'", m->guid);
        dictionary_destroy(m->machine_urls);

        debug(D_REGISTRY, "Registry: freeing machine '%s'", m->guid);
        freez(m);
    }

    // and free the memory of remaining dictionary structures

    debug(D_REGISTRY, "Registry: destroying persons dictionary");
    dictionary_destroy(registry.persons);

    debug(D_REGISTRY, "Registry: destroying machines dictionary");
    dictionary_destroy(registry.machines);
}

