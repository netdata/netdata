#ifndef NETDATA_PLUGIN_FREEBSD_H
#define NETDATA_PLUGIN_FREEBSD_H 1

#define GETSYSCTL(name, var) getsysctl(name, &(var), sizeof(var))

void *freebsd_main(void *ptr);

int getsysctl(const char *name, void *ptr, size_t len);

extern int do_freebsd_sysctl(int update_every, usec_t dt);

#endif /* NETDATA_PLUGIN_FREEBSD_H */
