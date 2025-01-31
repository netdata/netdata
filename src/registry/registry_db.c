// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "registry_internals.h"

int registry_db_should_be_saved(void) {
    netdata_log_debug(D_REGISTRY, "log entries %llu, max %llu", registry.log_count, registry.save_registry_every_entries);
    return registry.log_count > registry.save_registry_every_entries;
}

// ----------------------------------------------------------------------------
// INTERNAL FUNCTIONS FOR SAVING REGISTRY OBJECTS

static int registry_machine_save_url(REGISTRY_MACHINE_URL *mu, FILE *fp) {
    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_save_url('%s')", string2str(mu->url));

    int ret = fprintf(fp, "V\t%08x\t%08x\t%08x\t%02x\t%s\n",
            mu->first_t,
            mu->last_t,
            mu->usages,
            mu->flags,
            string2str(mu->url)
    );

    // error handling is done at registry_db_save()

    return ret;
}

static int registry_machine_save(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *file) {

    REGISTRY_MACHINE *m = entry;
    FILE *fp = file;

    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_machine_save('%s')", m->guid);

    int ret = fprintf(fp, "M\t%08x\t%08x\t%08x\t%s\n",
            m->first_t,
            m->last_t,
            m->usages,
            m->guid
    );

    if(ret >= 0) {
        for(REGISTRY_MACHINE_URL *mu = m->machine_urls; mu ; mu = mu->next) {
            int rc = registry_machine_save_url(mu, fp);
            if(rc < 0)
                return rc;

            ret += rc;
        }
    }

    // error handling is done at registry_db_save()

    return ret;
}

static inline int registry_person_save_url(REGISTRY_PERSON_URL *pu, FILE *fp) {
    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_person_save_url('%s')", string2str(pu->url));

    int ret = fprintf(fp, "U\t%08x\t%08x\t%08x\t%02x\t%s\t%s\t%s\n",
            pu->first_t,
            pu->last_t,
            pu->usages,
            pu->flags,
            pu->machine->guid,
            string2str(pu->machine_name),
            string2str(pu->url)
    );

    // error handling is done at registry_db_save()

    return ret;
}

static inline int registry_person_save(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *file) {
    REGISTRY_PERSON *p = entry;
    FILE *fp = file;

    netdata_log_debug(D_REGISTRY, "REGISTRY: registry_person_save('%s')", p->guid);

    int ret = fprintf(fp, "P\t%08x\t%08x\t%08x\t%s\n",
            p->first_t,
            p->last_t,
            p->usages,
            p->guid
    );

    if(ret >= 0) {
        for(REGISTRY_PERSON_URL *pu = p->person_urls; pu ;pu = pu->next) {
            int rc = registry_person_save_url(pu, fp);
            if(rc < 0)
                return rc;
            else
                ret += rc;
        }
    }

    // error handling is done at registry_db_save()

    return ret;
}

// ----------------------------------------------------------------------------
// SAVE THE REGISTRY DATABASE

int registry_db_save(void) {
    if(unlikely(!registry.enabled))
        return -1;

    if(unlikely(!registry_db_should_be_saved()))
        return -2;

    nd_log_limits_unlimited();

    char tmp_filename[FILENAME_MAX + 1];
    char old_filename[FILENAME_MAX + 1];

    snprintfz(old_filename, FILENAME_MAX, "%s.old", registry.db_filename);
    snprintfz(tmp_filename, FILENAME_MAX, "%s.tmp", registry.db_filename);

    netdata_log_debug(D_REGISTRY, "REGISTRY: Creating file '%s'", tmp_filename);
    FILE *fp = fopen(tmp_filename, "w");
    if(!fp) {
        netdata_log_error("REGISTRY: Cannot create file: %s", tmp_filename);
        nd_log_limits_reset();
        return -1;
    }

    // dictionary_walkthrough_read() has its own locking, so this is safe to do

    netdata_log_debug(D_REGISTRY, "REGISTRY: saving all machines");
    int bytes1 = dictionary_walkthrough_read(registry.machines, registry_machine_save, fp);
    if(bytes1 < 0) {
        netdata_log_error("REGISTRY: Cannot save registry machines - return value %d", bytes1);
        fclose(fp);
        nd_log_limits_reset();
        return bytes1;
    }
    netdata_log_debug(D_REGISTRY, "REGISTRY: saving machines took %d bytes", bytes1);

    netdata_log_debug(D_REGISTRY, "Saving all persons");
    int bytes2 = dictionary_walkthrough_read(registry.persons, registry_person_save, fp);
    if(bytes2 < 0) {
        netdata_log_error("REGISTRY: Cannot save registry persons - return value %d", bytes2);
        fclose(fp);
        nd_log_limits_reset();
        return bytes2;
    }
    netdata_log_debug(D_REGISTRY, "REGISTRY: saving persons took %d bytes", bytes2);

    // save the totals
    fprintf(fp, "T\t%016llx\t%016llx\t%016llx\t%016llx\t%016llx\t%016llx\n",
            registry.persons_count,
            registry.machines_count,
            registry.usages_count + 1, // this is required - it is lost on db rotation
            0LLU, //registry.urls_count,
            registry.persons_urls_count,
            registry.machines_urls_count
    );

    fclose(fp);

    errno_clear();

    // remove the .old db
    netdata_log_debug(D_REGISTRY, "REGISTRY: Removing old db '%s'", old_filename);
    if(unlink(old_filename) == -1 && errno != ENOENT)
        netdata_log_error("REGISTRY: cannot remove old registry file '%s'", old_filename);

    // rename the db to .old
    netdata_log_debug(D_REGISTRY, "REGISTRY: Link current db '%s' to .old: '%s'", registry.db_filename, old_filename);
    if(link(registry.db_filename, old_filename) == -1 && errno != ENOENT)
        netdata_log_error("REGISTRY: cannot move file '%s' to '%s'. Saving registry DB failed!", registry.db_filename, old_filename);

    else {
        // remove the database (it is saved in .old)
        netdata_log_debug(D_REGISTRY, "REGISTRY: removing db '%s'", registry.db_filename);
        if (unlink(registry.db_filename) == -1 && errno != ENOENT)
            netdata_log_error("REGISTRY: cannot remove old registry file '%s'", registry.db_filename);

        // move the .tmp to make it active
        netdata_log_debug(D_REGISTRY, "REGISTRY: linking tmp db '%s' to active db '%s'", tmp_filename, registry.db_filename);
        if (link(tmp_filename, registry.db_filename) == -1) {
            netdata_log_error("REGISTRY: cannot move file '%s' to '%s'. Saving registry DB failed!", tmp_filename,
                    registry.db_filename);

            // move the .old back
            netdata_log_debug(D_REGISTRY, "REGISTRY: linking old db '%s' to active db '%s'", old_filename, registry.db_filename);
            if(link(old_filename, registry.db_filename) == -1)
                netdata_log_error("REGISTRY: cannot move file '%s' to '%s'. Recovering the old registry DB failed!", old_filename, registry.db_filename);
        }
        else {
            netdata_log_debug(D_REGISTRY, "REGISTRY: removing tmp db '%s'", tmp_filename);
            if(unlink(tmp_filename) == -1)
                netdata_log_error("REGISTRY: cannot remove tmp registry file '%s'", tmp_filename);

            // it has been moved successfully
            // discard the current registry log
            registry_log_recreate();
            registry.log_count = 0;
        }
    }

    // continue operations
    nd_log_limits_reset();

    return -1;
}

// ----------------------------------------------------------------------------
// LOAD THE REGISTRY DATABASE

size_t registry_db_load(void) {
    char *s, buf[4096 + 1];
    REGISTRY_PERSON *p = NULL;
    REGISTRY_MACHINE *m = NULL;
    STRING *u = NULL;
    size_t line = 0;

    netdata_log_debug(D_REGISTRY, "REGISTRY: loading active db from: '%s'", registry.db_filename);
    FILE *fp = fopen(registry.db_filename, "r");
    if(!fp) {
        if (errno != ENOENT)
            netdata_log_error("REGISTRY: cannot open registry file: '%s'", registry.db_filename);
        return 0;
    }

    REGISTRY_MACHINE_URL *mu;
    size_t len = 0;
    buf[4096] = '\0';
    while((s = fgets_trim_len(buf, 4096, fp, &len))) {
        line++;

        netdata_log_debug(D_REGISTRY, "REGISTRY: read line %zu to length %zu: %s", line, len, s);
        switch(*s) {
            case 'U': // person URL
                if(unlikely(!p)) {
                    netdata_log_error("REGISTRY: ignoring line %zu, no person loaded: %s", line, s);
                    continue;
                }

                // verify it is valid
                if(len < 69 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[31] != '\t' || s[68] != '\t') {
                    netdata_log_error("REGISTRY: person URL line %zu is wrong (len = %zu).", line, len);
                    continue;
                }

                s[1] = s[10] = s[19] = s[28] = s[31] = s[68] = '\0';

                // skip the name to find the url
                char *url = &s[69];
                while(*url && *url != '\t') url++;
                if(!*url) {
                    netdata_log_error("REGISTRY: person URL line %zu does not have a url.", line);
                    continue;
                }
                *url++ = '\0';

                if(*url != 'h' && *url != '*') {
                    netdata_log_error("REGISTRY: person URL line %zu does not have a valid url: %s", line, url);
                    continue;
                }

                u = string_strdupz(url);

                time_t first_t = (time_t)strtoul(&s[2], NULL, 16);

                m = registry_machine_find(&s[32]);
                if(!m) m = registry_machine_allocate(&s[32], first_t);

                mu = registry_machine_url_find(m, u);
                if(!mu) {
                    netdata_log_error("REGISTRY: person URL line %zu was not linked to the machine it refers to", line);
                    mu = registry_machine_url_allocate(m, u, first_t);
                }

                REGISTRY_PERSON_URL *pu = registry_person_url_index_find(p, u);
                if(!pu)
                    pu = registry_person_url_allocate(p, m, u, &s[69], strlen(&s[69]), first_t);
                else
                    netdata_log_error("REGISTRY: person URL line %zu is duplicate, reusing the old one.", line);

                pu->last_t = (uint32_t)strtoul(&s[11], NULL, 16);
                pu->usages = (uint32_t)strtoul(&s[20], NULL, 16);
                pu->flags = (uint8_t)strtoul(&s[29], NULL, 16);
                netdata_log_debug(D_REGISTRY, "REGISTRY: loaded person URL '%s' with name '%s' of machine '%s', first: %u, last: %u, usages: %u, flags: %02x",
                      string2str(u), string2str(pu->machine_name), m->guid, pu->first_t, pu->last_t, pu->usages, pu->flags);

                string_freez(u);
                break;

            case 'P': // person
                m = NULL;
                // verify it is valid
                if(unlikely(len != 65 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[65] != '\0')) {
                    netdata_log_error("REGISTRY: person line %zu is wrong (len = %zu).", line, len);
                    continue;
                }

                s[1] = s[10] = s[19] = s[28] = '\0';
                p = registry_person_allocate(&s[29], (time_t)strtoul(&s[2], NULL, 16));
                p->last_t = (uint32_t)strtoul(&s[11], NULL, 16);
                p->usages = (uint32_t)strtoul(&s[20], NULL, 16);
                netdata_log_debug(D_REGISTRY, "REGISTRY: loaded person '%s', first: %u, last: %u, usages: %u", p->guid, p->first_t, p->last_t, p->usages);
                break;

            case 'V': // machine URL
                if(unlikely(!m)) {
                    netdata_log_error("REGISTRY: ignoring line %zu, no machine loaded: %s", line, s);
                    continue;
                }

                // verify it is valid
                if(len < 32 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[31] != '\t') {
                    netdata_log_error("REGISTRY: person URL line %zu is wrong (len = %zu).", line, len);
                    continue;
                }

                s[1] = s[10] = s[19] = s[28] = s[31] = '\0';

                url = &s[32];
                if(*url != 'h' && *url != '*') {
                    netdata_log_error("REGISTRY: machine URL line %zu does not have a valid url: %s", line, url);
                    continue;
                }

                u = string_strdupz(url);

                mu = registry_machine_url_find(m, u);
                if(!mu)
                    mu = registry_machine_url_allocate(m, u, (time_t)strtoul(&s[2], NULL, 16));
                else
                    netdata_log_error("REGISTRY: machine URL line %zu is duplicate, reusing the old one.", line);

                mu->last_t = (uint32_t)strtoul(&s[11], NULL, 16);
                mu->usages = (uint32_t)strtoul(&s[20], NULL, 16);
                mu->flags = (uint8_t)strtoul(&s[29], NULL, 16);
                netdata_log_debug(D_REGISTRY, "Registry loaded machine URL '%s', machine '%s', first: %u, last: %u, usages: %u, flags: %02x",
                      string2str(u), m->guid, mu->first_t, mu->last_t, mu->usages, mu->flags);

                string_freez(u);
                break;

            case 'M': // machine
                p = NULL;
                // verify it is valid
                if(unlikely(len != 65 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[65] != '\0')) {
                    netdata_log_error("REGISTRY: person line %zu is wrong (len = %zu).", line, len);
                    continue;
                }

                s[1] = s[10] = s[19] = s[28] = '\0';
                m = registry_machine_allocate(&s[29], (time_t)strtoul(&s[2], NULL, 16));
                m->last_t = (uint32_t)strtoul(&s[11], NULL, 16);
                m->usages = (uint32_t)strtoul(&s[20], NULL, 16);
                netdata_log_debug(D_REGISTRY, "REGISTRY: loaded machine '%s', first: %u, last: %u, usages: %u", m->guid, m->first_t, m->last_t, m->usages);
                break;

            case 'T': // totals
                if(unlikely(len != 103 || s[1] != '\t' || s[18] != '\t' || s[35] != '\t' || s[52] != '\t' || s[69] != '\t' || s[86] != '\t' || s[103] != '\0')) {
                    netdata_log_error("REGISTRY: totals line %zu is wrong (len = %zu).", line, len);
                    continue;
                }
                registry.persons_count = strtoull(&s[2], NULL, 16);
                registry.machines_count = strtoull(&s[19], NULL, 16);
                registry.usages_count = strtoull(&s[36], NULL, 16);
                registry.persons_urls_count = strtoull(&s[70], NULL, 16);
                registry.machines_urls_count = strtoull(&s[87], NULL, 16);
                break;

            default:
                netdata_log_error("REGISTRY: ignoring line %zu of filename '%s': %s.", line, registry.db_filename, s);
                break;
        }
    }
    fclose(fp);

    return line;
}
