// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
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
    registry.pathname = config_get(CONFIG_SECTION_DIRECTORIES, "registry", filename);
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
    registry.enable_cookies_samesite_secure = config_get_boolean(CONFIG_SECTION_REGISTRY, "enable cookies SameSite and Secure", 1);

    registry_update_cloud_base_url();
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
    registry.persons = dictionary_create(REGISTRY_DICTIONARY_OPTIONS);
    registry.machines = dictionary_create(REGISTRY_DICTIONARY_OPTIONS);
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

static int machine_urls_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *data) {
    REGISTRY_MACHINE *m = (REGISTRY_MACHINE *)data;
    (void)m;

    REGISTRY_MACHINE_URL *mu = (REGISTRY_MACHINE_URL *)entry;

    debug(D_REGISTRY, "Registry: unlinking url '%s' from machine", mu->url->url);
    registry_url_unlink(mu->url);

    debug(D_REGISTRY, "Registry: freeing machine url");
    freez(mu);

    return 1;
}

static int machine_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *data __maybe_unused) {
    REGISTRY_MACHINE *m = (REGISTRY_MACHINE *)entry;
    int ret = dictionary_walkthrough_read(m->machine_urls, machine_urls_delete_callback, m);

    dictionary_destroy(m->machine_urls);
    freez(m);

    return ret + 1;
}
static int registry_person_del_callback(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *d __maybe_unused) {
    REGISTRY_PERSON *p = (REGISTRY_PERSON *)entry;

    debug(D_REGISTRY, "Registry: registry_person_del('%s'): deleting person", p->guid);

    while(p->person_urls.root)
        registry_person_unlink_from_url(p, (REGISTRY_PERSON_URL *)p->person_urls.root);

    //debug(D_REGISTRY, "Registry: deleting person '%s' from persons registry", p->guid);
    //dictionary_del(registry.persons, p->guid);

    debug(D_REGISTRY, "Registry: freeing person '%s'", p->guid);
    freez(p);

    return 1;
}

void registry_free(void) {
    if(!registry.enabled) return;

    debug(D_REGISTRY, "Registry: destroying persons dictionary");
    dictionary_walkthrough_read(registry.persons, registry_person_del_callback, NULL);
    dictionary_destroy(registry.persons);

    debug(D_REGISTRY, "Registry: destroying machines dictionary");
    dictionary_walkthrough_read(registry.machines, machine_delete_callback, NULL);
    dictionary_destroy(registry.machines);
}
