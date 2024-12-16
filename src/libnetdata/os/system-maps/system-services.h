// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_SERVICES_H
#define NETDATA_SYSTEM_SERVICES_H

#include "libnetdata/libnetdata.h"
#include <netdb.h>

// --------------------------------------------------------------------------------------------------------------------
// hashtable for caching port and protocol to service name mappings
// key is the combination of protocol and port packed into an uint64_t, value is service name (STRING)

#define SIMPLE_HASHTABLE_VALUE_TYPE STRING *
#define SIMPLE_HASHTABLE_NAME _SERVICENAMES_CACHE
#include "libnetdata/simple_hashtable/simple_hashtable.h"

typedef struct servicenames_cache {
    SPINLOCK spinlock;
    SIMPLE_HASHTABLE_SERVICENAMES_CACHE ht;
} SERVICENAMES_CACHE;

static inline const char *system_servicenames_ipproto2str(uint16_t ipproto) {
    return (ipproto == IPPROTO_TCP) ? "tcp" : "udp";
}

static inline const char *static_portnames(uint16_t port, uint16_t ipproto) {
    if(port == 19999 && ipproto == IPPROTO_TCP)
        return "netdata";

    if(port == 8125)
        return "statsd";

    return NULL;
}

static inline STRING *system_servicenames_cache_lookup(SERVICENAMES_CACHE *sc, uint16_t port, uint16_t ipproto) {
    struct {
        uint16_t ipproto;
        uint16_t port;
    } key = {
        .ipproto = ipproto,
        .port = port,
    };
    XXH64_hash_t hash = XXH3_64bits(&key, sizeof(key));

    spinlock_lock(&sc->spinlock);

    SIMPLE_HASHTABLE_SLOT_SERVICENAMES_CACHE *sl = simple_hashtable_get_slot_SERVICENAMES_CACHE(&sc->ht, hash, &key, true);
    STRING *s = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if (!s) {
        const char *st = static_portnames(port, ipproto);
        if(st) {
            s = string_strdupz(st);
        }
        else {
            struct servent *se = getservbyport(htons(port), system_servicenames_ipproto2str(ipproto));

            if (!se || !se->s_name) {
                char name[50];
                snprintfz(name, sizeof(name), "%u/%s", port, system_servicenames_ipproto2str(ipproto));
                s = string_strdupz(name);
            }
            else
                s = string_strdupz(se->s_name);
        }

        simple_hashtable_set_slot_SERVICENAMES_CACHE(&sc->ht, sl, hash, s);
    }

    s = string_dup(s);
    spinlock_unlock(&sc->spinlock);
    return s;
}

static inline SERVICENAMES_CACHE *system_servicenames_cache_init(void) {
    SERVICENAMES_CACHE *sc = callocz(1, sizeof(*sc));
    spinlock_init(&sc->spinlock);
    simple_hashtable_init_SERVICENAMES_CACHE(&sc->ht, 100);
    return sc;
}

static inline void system_servicenames_cache_destroy(SERVICENAMES_CACHE *sc) {
    spinlock_lock(&sc->spinlock);

    for (SIMPLE_HASHTABLE_SLOT_SERVICENAMES_CACHE *sl = simple_hashtable_first_read_only_SERVICENAMES_CACHE(&sc->ht);
         sl;
         sl = simple_hashtable_next_read_only_SERVICENAMES_CACHE(&sc->ht, sl)) {
        STRING *s = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        string_freez(s);
    }

    simple_hashtable_destroy_SERVICENAMES_CACHE(&sc->ht);
    freez(sc);
}

#endif //NETDATA_SYSTEM_SERVICES_H
