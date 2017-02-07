#ifndef NETDATA_PLUGIN_MACOS_H
#define NETDATA_PLUGIN_MACOS_H 1

/**
 * @file plugin_macos.h
 * @brief Thread to collect metrics of macos.
 */

/**
 * Main method of macos thread.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
void *macos_main(void *ptr);

/**
 * sysctlbyname() with error logging.
 *
 * @param name to query
 * @param var to store result
 * @return 0 on success, 1 on error
 */
#define GETSYSCTL(name, var) getsysctl(name, &(var), sizeof(var))

/**
 * sysctlbyname() with error logging.
 *
 * Use GETSYSCTL in favor of this. 
 *
 * @param name to query
 * @param ptr to store result
 * @param len of `ptr`
 * @return 0 on success. 1 on error.
 */
extern int getsysctl(const char *name, void *ptr, size_t len);


/**
 * Sysctl data collector of macOS.
 *
 * Collect data with sysctl on macOS.
 *
 * This is called by `freebsd_main` every `update_every` second.
 * This function should push values to the round robin database.
 *
 * @param update_every Intervall in seconds this is called.
 * @param dt Microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_macos_sysctl(int update_every, usec_t dt);
/**
 * Collect system data of macOS.
 *
 * Collect data with `host_statistics64()` on macOS
 *
 * This is called by `freebsd_main` every `update_every` second.
 * This function should push values to the round robin database.
 *
 * @param update_every Intervall in seconds this is called.
 * @param dt Microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_macos_mach_smi(int update_every, usec_t dt);
/**
 * Collect disk statistics of macOS.
 *
 * This is called by `freebsd_main` every `update_every` second.
 * This function should push values to the round robin database.
 *
 * @param update_every Intervall in seconds this is called.
 * @param dt Microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_macos_iokit(int update_every, usec_t dt);

#endif /* NETDATA_PLUGIN_MACOS_H */
