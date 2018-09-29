// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "registry_internals.h"

void registry_log(char action, REGISTRY_PERSON *p, REGISTRY_MACHINE *m, REGISTRY_URL *u, char *name) {
    if(likely(registry.log_fp)) {
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

        // this must be outside the log_lock(), or a deadlock will happen.
        // registry_db_save() checks the same inside the log_lock, so only
        // one thread will save the db
        if(unlikely(registry_db_should_be_saved()))
            registry_db_save();
    }
}

int registry_log_open(void) {
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

void registry_log_close(void) {
    if(registry.log_fp) {
        fclose(registry.log_fp);
        registry.log_fp = NULL;
    }
}

void registry_log_recreate(void) {
    if(registry.log_fp != NULL) {
        registry_log_close();

        // open it with truncate
        registry.log_fp = fopen(registry.log_filename, "w");
        if(registry.log_fp) fclose(registry.log_fp);
        else error("Cannot truncate registry log '%s'", registry.log_filename);

        registry.log_fp = NULL;
        registry_log_open();
    }
}

ssize_t registry_log_load(void) {
    ssize_t line = -1;

    // closing the log is required here
    // otherwise we will append to it the values we read
    registry_log_close();

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
                    REGISTRY_PERSON *p = registry_person_find(person_guid);
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
    registry_log_open();

    return line;
}
