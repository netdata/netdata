// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "registry_internals.h"

void registry_db_stats(void) {
    size_t persons = 0;
    size_t persons_urls = 0;
    size_t max_urls_per_person = 0;

    REGISTRY_PERSON *p;
    dfe_start_read(registry.persons, p) {
        persons++;
        size_t urls = 0;
        for(REGISTRY_PERSON_URL *pu = p->person_urls ; pu ;pu = pu->next)
            urls++;

        if(urls > max_urls_per_person)
            max_urls_per_person = urls;

        persons_urls += urls;
    }
    dfe_done(p);

    size_t machines = 0;
    size_t machines_urls = 0;
    size_t max_urls_per_machine = 0;

    REGISTRY_MACHINE *m;
    dfe_start_read(registry.machines, m) {
                machines++;
                size_t urls = 0;
                for(REGISTRY_MACHINE_URL *mu = m->machine_urls ; mu ;mu = mu->next)
                    urls++;

                if(urls > max_urls_per_machine)
                    max_urls_per_machine = urls;

                machines_urls += urls;
            }
    dfe_done(m);

    netdata_log_info("REGISTRY: persons %zu, person_urls %zu, max_urls_per_person %zu, "
                     "machines %zu, machine_urls %zu, max_urls_per_machine %zu",
                     persons, persons_urls, max_urls_per_person,
                     machines, machines_urls, max_urls_per_machine);
}

void registry_generate_curl_urls(void) {
    FILE *fp = fopen("/tmp/registry.curl", "w+");
    if (unlikely(!fp))
        return;

    REGISTRY_PERSON *p;
    dfe_start_read(registry.persons, p) {
        for(REGISTRY_PERSON_URL *pu = p->person_urls ; pu ;pu = pu->next) {
            fprintf(fp, "do_curl '%s' '%s' '%s'\n", p->guid, pu->machine->guid, string2str(pu->url));
        }
    }
    dfe_done(p);

    fclose(fp);
}

void registry_init(void) {
    FUNCTION_RUN_ONCE();

    netdata_conf_section_global();

    char filename[FILENAME_MAX + 1];

    // registry enabled?
    if(web_server_mode != WEB_SERVER_MODE_NONE) {
        registry.enabled = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_REGISTRY, "enabled", 0);
    }
    else {
        netdata_log_info("Registry is disabled");
        inicfg_set_boolean(&netdata_config, CONFIG_SECTION_REGISTRY, "enabled", 0);
        registry.enabled = 0;
    }

    // path names
    snprintfz(filename, FILENAME_MAX, "%s/registry", netdata_configured_varlib_dir);
    registry.pathname = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "registry", filename);
    verify_required_directory(NULL, registry.pathname, true, 0770);

    // filenames
    snprintfz(filename, FILENAME_MAX, "%s/netdata.public.unique.id", registry.pathname);
    registry.machine_guid_filename = inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "netdata unique id file", filename);

    snprintfz(filename, FILENAME_MAX, "%s/registry.db", registry.pathname);
    registry.db_filename = inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "registry db file", filename);

    snprintfz(filename, FILENAME_MAX, "%s/registry-log.db", registry.pathname);
    registry.log_filename = inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "registry log file", filename);

    // configuration options
    registry.save_registry_every_entries = (unsigned long long)inicfg_get_number(&netdata_config, CONFIG_SECTION_REGISTRY, "registry save db every new entries", 1000000);
    registry.persons_expiration = inicfg_get_duration_days_to_seconds(
                                      &netdata_config, CONFIG_SECTION_REGISTRY, "registry expire idle persons", 365 * 86400);
    registry.registry_domain = inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "registry domain", "");
    registry.registry_to_announce = inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "registry to announce", "https://registry.my-netdata.io");
    registry.hostname = inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "registry hostname", netdata_configured_hostname);
    registry.verify_cookies_redirects = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_REGISTRY, "verify browser cookies support", 1);
    registry.enable_cookies_samesite_secure = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_REGISTRY, "enable cookies SameSite and Secure", 1);

    registry_update_cloud_base_url();
    nd_setenv("NETDATA_REGISTRY_HOSTNAME", registry.hostname, 1);
    nd_setenv("NETDATA_REGISTRY_URL", registry.registry_to_announce, 1);

    registry.max_url_length = (size_t)inicfg_get_number(&netdata_config, CONFIG_SECTION_REGISTRY, "max URL length", 1024);
    if(registry.max_url_length < 10) {
        registry.max_url_length = 10;
        inicfg_set_number(&netdata_config, CONFIG_SECTION_REGISTRY, "max URL length", (long long)registry.max_url_length);
    }

    registry.max_name_length = (size_t)inicfg_get_number(&netdata_config, CONFIG_SECTION_REGISTRY, "max URL name length", 50);
    if(registry.max_name_length < 10) {
        registry.max_name_length = 10;
        inicfg_set_number(&netdata_config, CONFIG_SECTION_REGISTRY, "max URL name length", (long long)registry.max_name_length);
    }

    // initialize entries counters
    registry.persons_count = 0;
    registry.machines_count = 0;
    registry.usages_count = 0;
    registry.persons_urls_count = 0;
    registry.machines_urls_count = 0;

    // initialize locks
    netdata_mutex_init(&registry.lock);
}

bool registry_load(void) {
    if(registry.enabled) {
        bool use_mmap = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_REGISTRY, "use mmap", false);

        // create dictionaries
        registry.persons = dictionary_create(REGISTRY_DICTIONARY_OPTIONS);
        registry.machines = dictionary_create(REGISTRY_DICTIONARY_OPTIONS);

        // initialize the allocators

        size_t min_page_size = 4 * 1024;
        size_t max_page_size = 1024 * 1024;

        if(use_mmap) {
            min_page_size = 100 * 1024 * 1024;
            max_page_size = 512 * 1024 * 1024;
        }

        registry.persons_aral = aral_create("registry_persons", sizeof(REGISTRY_PERSON),
                                            min_page_size / sizeof(REGISTRY_PERSON), max_page_size,
                                            &registry.aral_stats,
                                            "registry_persons",
                                            &netdata_configured_cache_dir,
                                            use_mmap, true, true);

        registry.machines_aral = aral_create("registry_machines", sizeof(REGISTRY_MACHINE),
                                             min_page_size / sizeof(REGISTRY_MACHINE), max_page_size,
                                             &registry.aral_stats,
                                             "registry_machines",
                                             &netdata_configured_cache_dir,
                                             use_mmap, true, true);

        registry.person_urls_aral = aral_create("registry_person_urls", sizeof(REGISTRY_PERSON_URL),
                                                min_page_size / sizeof(REGISTRY_PERSON_URL), max_page_size,
                                                &registry.aral_stats,
                                                "registry_person_urls",
                                                &netdata_configured_cache_dir,
                                                use_mmap, true, true);

        registry.machine_urls_aral = aral_create("registry_machine_urls", sizeof(REGISTRY_MACHINE_URL),
                                                 min_page_size / sizeof(REGISTRY_MACHINE_URL), max_page_size,
                                                 &registry.aral_stats,
                                                 "registry_machine_urls",
                                                 &netdata_configured_cache_dir,
                                                 use_mmap, true, true);

        registry_log_open();
        registry_db_load();
        registry_log_load();

        if(unlikely(registry_db_should_be_saved()))
            registry_db_save();

        //        registry_db_stats();
        //        registry_generate_curl_urls();
        //        exit(0);

        return true;
    }

    return false;
}

static int machine_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *data __maybe_unused) {
    REGISTRY_MACHINE *m = (REGISTRY_MACHINE *)entry;

    int count = 0;

    while(m->machine_urls) {
        registry_machine_url_unlink_from_machine_and_free(m, m->machine_urls);
        count++;
    }

    aral_freez(registry.machines_aral, m);

    return count + 1;
}

static int registry_person_del_callback(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *d __maybe_unused) {
    REGISTRY_PERSON *p = (REGISTRY_PERSON *)entry;

    netdata_log_debug(D_REGISTRY, "Registry: registry_person_del('%s'): deleting person", p->guid);

    while(p->person_urls)
        registry_person_unlink_from_url(p, (REGISTRY_PERSON_URL *)p->person_urls);

    //debug(D_REGISTRY, "Registry: deleting person '%s' from persons registry", p->guid);
    //dictionary_del(registry.persons, p->guid);

    netdata_log_debug(D_REGISTRY, "Registry: freeing person '%s'", p->guid);
    aral_freez(registry.persons_aral, p);

    return 1;
}

void registry_free(void) {
    if(!registry.enabled) return;
    registry.enabled = false;

    netdata_log_debug(D_REGISTRY, "Registry: destroying persons dictionary");
    dictionary_walkthrough_read(registry.persons, registry_person_del_callback, NULL);
    dictionary_destroy(registry.persons);
    registry.persons = NULL;

    netdata_log_debug(D_REGISTRY, "Registry: destroying machines dictionary");
    dictionary_walkthrough_read(registry.machines, machine_delete_callback, NULL);
    dictionary_destroy(registry.machines);
    registry.machines = NULL;

    aral_destroy(registry.persons_aral);
    aral_destroy(registry.machines_aral);
    aral_destroy(registry.person_urls_aral);
    aral_destroy(registry.machine_urls_aral);
    registry.persons_aral = NULL;
    registry.machines_aral = NULL;
    registry.person_urls_aral = NULL;
    registry.machine_urls_aral = NULL;
}
