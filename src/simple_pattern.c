#include "common.h"

struct simple_pattern {
    const char *match;
    size_t len;
    NETDATA_SIMPLE_PREFIX_MODE mode;

    struct simple_pattern *child;

    struct simple_pattern *next;
};

static inline struct simple_pattern *parse_pattern(const char *str, NETDATA_SIMPLE_PREFIX_MODE default_mode) {
    /*
     * DEBUG
    info(">>>> PARSE: '%s'", str);
     */

    NETDATA_SIMPLE_PREFIX_MODE mode;
    struct simple_pattern *child = NULL;

    char *buf = strdupz(str);
    char *s = buf, *c = buf;

    // skip asterisks in front
    while(*c == '*') c++;

    // find the next asterisk
    while(*c && *c != '*') c++ ;

    // do we have an asterisk in the middle?
    if(*c == '*' && c[1] != '\0') {
        // yes, we have
        child = parse_pattern(c, default_mode);
        c[1] = '\0';
    }

    // check what this one matches

    size_t len = strlen(s);
    if(len >= 2 && *s == '*' && s[len - 1] == '*') {
        s[len - 1] = '\0';
        s++;
        mode = NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING;
    }
    else if(len >= 1 && *s == '*') {
        s++;
        mode = NETDATA_SIMPLE_PATTERN_MODE_SUFFIX;
    }
    else if(len >= 1 && s[len - 1] == '*') {
        s[len - 1] = '\0';
        mode = NETDATA_SIMPLE_PATTERN_MODE_PREFIX;
    }
    else
        mode = default_mode;

    // allocate the structure
    struct simple_pattern *m = callocz(1, sizeof(struct simple_pattern));
    if(*s) {
        m->match = strdup(s);
        m->len = strlen(m->match);
        m->mode = mode;
    }
    else {
        m->mode = NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING;
    }

    m->child = child;

    free(buf);

    /*
     * DEBUG
    info("PATTERN '%s' is composed by", str);
    struct simple_pattern *p;
    for(p = m; p ; p = p->child)
        info(">>>> COMPONENT: '%s%s%s' (len %zu type %u)",
            (p->mode == NETDATA_SIMPLE_PATTERN_MODE_SUFFIX || p->mode == NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING)?"*":"",
            p->match,
            (p->mode == NETDATA_SIMPLE_PATTERN_MODE_PREFIX || p->mode == NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING)?"*":"",
            p->len,
            p->mode);
     */

    return m;
}

NETDATA_SIMPLE_PATTERN *netdata_simple_pattern_list_create(const char *list, NETDATA_SIMPLE_PREFIX_MODE default_mode) {
    struct simple_pattern *root = NULL, *last = NULL;

    if(unlikely(!list || !*list)) return root;

    char *buf = strdupz(list);
    if(buf && *buf) {
        char *s = buf;

        while(s && *s) {
            // skip all spaces
            while(isspace(*s)) s++;

            // empty string
            if(unlikely(!*s)) break;

            // find the next space
            char *c = s;
            while(*c && !isspace(*c)) c++;

            // find the next word
            char *n;
            if(likely(*c)) n = c + 1;
            else n = NULL;

            // terminate our string
            *c = '\0';

            struct simple_pattern *m = parse_pattern(s, default_mode);

            if(likely(n)) *c = ' ';

            // link it at the end
            if(unlikely(!root))
                root = last = m;
            else {
                last->next = m;
                last = m;
            }

            // prepare for next loop
            s = n;
        }
    }

    free(buf);
    return (NETDATA_SIMPLE_PATTERN *)root;
}

static inline int match_pattern(struct simple_pattern *m, const char *str, size_t len) {
    /*
     * DEBUG
     *
    info("CHECK string '%s' (len %zu) with pattern '%s%s%s' (len %zu type %u)", str, len,
            (m->mode == NETDATA_SIMPLE_PATTERN_MODE_SUFFIX || m->mode == NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING)?"*":"",
            m->match,
            (m->mode == NETDATA_SIMPLE_PATTERN_MODE_PREFIX || m->mode == NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING)?"*":"",
            m->len, m->mode);
    */

    char *s;

    if(m->len <= len) {
        switch(m->mode) {
            case NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING:
                if(!m->len) return 1;
                if((s = strstr(str, m->match))) {
                    if(!m->child) return 1;
                    return match_pattern(m->child, &s[m->len], len - (s - str) - m->len);
                }
                break;

            case NETDATA_SIMPLE_PATTERN_MODE_PREFIX:
                if(unlikely(strncmp(str, m->match, m->len) == 0)) {
                    if(!m->child) return 1;
                    return match_pattern(m->child, &str[m->len], len - m->len);
                }
                break;

            case NETDATA_SIMPLE_PATTERN_MODE_SUFFIX:
                if(unlikely(strcmp(&str[len - m->len], m->match) == 0)) {
                    if(!m->child) return 1;
                    return 0;
                }
                break;

            case NETDATA_SIMPLE_PATTERN_MODE_EXACT:
            default:
                if(unlikely(strcmp(str, m->match) == 0)) {
                    if(!m->child) return 1;
                    return 0;
                }
                break;
        }
    }

    return 0;
}

int netdata_simple_pattern_list_matches(NETDATA_SIMPLE_PATTERN *list, const char *str) {
    struct simple_pattern *m, *root = (struct simple_pattern *)list;

    if(unlikely(!root)) return 0;

    size_t len = strlen(str);
    for(m = root; m ; m = m->next)
        if(match_pattern(m, str, len)) {
            /*
             * DEBUG
             *
            info("MATCHED string '%s' (len %zu) with pattern '%s%s%s' (len %zu type %u)", str, len,
                    (m->mode == NETDATA_SIMPLE_PATTERN_MODE_SUFFIX || m->mode == NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING)?"*":"",
                    m->match,
                    (m->mode == NETDATA_SIMPLE_PATTERN_MODE_PREFIX || m->mode == NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING)?"*":"",
                    m->len, m->mode);

            struct simple_pattern *p;
            for(p = m; p ; p = p->child)
                info(">>>> MATCHED COMPONENT: '%s%s%s' (len %zu type %u)",
                        (p->mode == NETDATA_SIMPLE_PATTERN_MODE_SUFFIX || p->mode == NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING)?"*":"",
                        p->match,
                        (p->mode == NETDATA_SIMPLE_PATTERN_MODE_PREFIX || p->mode == NETDATA_SIMPLE_PATTERN_MODE_SUBSTRING)?"*":"",
                        p->len,
                        p->mode);
            */

            return 1;
        }

    return 0;
}

static inline void free_pattern(struct simple_pattern *m) {
    if(m->next) free_pattern(m->next);
    if(m->child) free_pattern(m->child);
    freez((void *)m->match);
    freez(m);
}

void netdata_simple_pattern_free(NETDATA_SIMPLE_PATTERN *list) {
    free_pattern(((struct simple_pattern *)list)->next);
}
