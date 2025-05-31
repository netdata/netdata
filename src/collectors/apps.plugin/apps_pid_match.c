// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

bool pid_match_check(struct pid_stat *p, APPS_MATCH *match) {
    if(!match->starts_with && !match->ends_with) {
        // Exact match
        if(match->pattern) {
            if(simple_pattern_matches_string(match->pattern, p->comm))
                return true;
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
            if(p->name && simple_pattern_matches_string(match->pattern, p->name))
                return true;
#endif
        }
        else {
            if(string_equals_string_nocase(match->compare, p->comm) || 
               string_equals_string_nocase(match->compare, p->comm_orig))
                return true;
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
            if(p->name && string_equals_string_nocase(match->compare, p->name))
                return true;
#endif
        }
    }
    else if(match->starts_with && !match->ends_with) {
        // Prefix match
        if(match->pattern) {
            if(simple_pattern_matches_string(match->pattern, p->comm))
                return true;
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
            if(p->name && simple_pattern_matches_string(match->pattern, p->name))
                return true;
#endif
        }
        else {
            if(string_starts_with_string_nocase(p->comm, match->compare) ||
               (p->comm != p->comm_orig && string_starts_with_string_nocase(p->comm_orig, match->compare)))
                return true;
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
            if(p->name && string_starts_with_string_nocase(p->name, match->compare))
                return true;
#endif
        }
    }
    else if(!match->starts_with && match->ends_with) {
        // Suffix match
        if(match->pattern) {
            if(simple_pattern_matches_string(match->pattern, p->comm))
                return true;
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
            if(p->name && simple_pattern_matches_string(match->pattern, p->name))
                return true;
#endif
        }
        else {
            if(string_ends_with_string_nocase(p->comm, match->compare) ||
               (p->comm != p->comm_orig && string_ends_with_string_nocase(p->comm_orig, match->compare)))
                return true;
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
            if(p->name && string_ends_with_string_nocase(p->name, match->compare))
                return true;
#endif
        }
    }
    else if(match->starts_with && match->ends_with) {
        // Substring match - search in cmdline and on Windows also in name
        if(p->cmdline) {
            if(match->pattern) {
                if(simple_pattern_matches_string(match->pattern, p->cmdline))
                    return true;
            }
            else {
                if(strcasestr(string2str(p->cmdline), string2str(match->compare)))
                    return true;
            }
        }
#if (PROCESSES_HAVE_COMM_AND_NAME == 1)
        // On Windows, also search in the name field for substring patterns
        if(p->name) {
            if(match->pattern) {
                if(simple_pattern_matches_string(match->pattern, p->name))
                    return true;
            }
            else {
                if(strcasestr(string2str(p->name), string2str(match->compare)))
                    return true;
            }
        }
#endif
    }

    return false;
}

APPS_MATCH pid_match_create(const char *comm) {
    APPS_MATCH m = {
            .starts_with = false,
            .ends_with = false,
            .compare = NULL,
            .pattern = NULL,
    };

    // copy comm to make changes to it
    size_t len = strlen(comm);
    char buf[len + 1];
    memcpy(buf, comm, sizeof(buf));

    trim_all(buf);

    if(buf[len - 1] == '*') {
        buf[--len] = '\0';
        m.starts_with = true;
    }

    const char *nid = buf;
    if (nid[0] == '*') {
        m.ends_with = true;
        nid++;
    }

    m.compare = string_strdupz(nid);

    if(strchr(nid, '*'))
        m.pattern = simple_pattern_create(comm, SIMPLE_PATTERN_NO_SEPARATORS, SIMPLE_PATTERN_EXACT, false);

    return m;
}

void pid_match_cleanup(APPS_MATCH *m) {
    string_freez(m->compare);
    simple_pattern_free(m->pattern);
}

