#ifndef NETDATA_PLUGIN_FREEBSD_H
#define NETDATA_PLUGIN_FREEBSD_H 1

/**
 * @file plugin_freebsd.h
 * @brief Thread to collect metrics of freebsd.
 */

#include <sys/sysctl.h>

/**
 * sysctlbyname() with error logging.
 *
 * @param name to query
 * @param var to store result
 * @return 0 on success. 1 on error.
 */
#define GETSYSCTL(name, var) getsysctl(name, &(var), sizeof(var))

/**
 * Main function of freebsd thread.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
void *freebsd_main(void *ptr);

/**
 * Data collector of freebsd.
 *
 * Function doing the data collection for freebsd.
 * This is called by `freebsd_main` every `update_every` second.
 * This function should push values to the round robin database.
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_freebsd_sysctl(int update_every, usec_t dt);

static inline int getsysctl(const char *name, void *ptr, size_t len)
{
    size_t nlen = len;

    if (unlikely(sysctlbyname(name, ptr, &nlen, NULL, 0) == -1)) {
        error("FREEBSD: sysctl(%s...) failed: %s", name, strerror(errno));
        return 1;
    }
    if (unlikely(nlen != len)) {
        error("FREEBSD: sysctl(%s...) expected %lu, got %lu", name, (unsigned long)len, (unsigned long)nlen);
        return 1;
    }
    return 0;
}

#endif /* NETDATA_PLUGIN_FREEBSD_H */
