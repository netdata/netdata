#ifndef NETDATA_PLUGIN_MACOS_H
#define NETDATA_PLUGIN_MACOS_H 1

void *macos_main(void *ptr);

#define GETSYSCTL(name, var) getsysctl(name, &(var), sizeof(var))

extern int getsysctl(const char *name, void *ptr, size_t len);

extern int do_macos_sysctl(int update_every, usec_t dt);
extern int do_macos_mach_smi(int update_every, usec_t dt);
extern int do_macos_iokit(int update_every, usec_t dt);

#endif /* NETDATA_PLUGIN_MACOS_H */
