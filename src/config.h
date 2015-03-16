#include "web_buffer.h"

#ifndef NETDATA_CONFIG_H
#define NETDATA_CONFIG_H 1

#define CONFIG_MAX_NAME 100
#define CONFIG_MAX_VALUE 1024
#define CONFIG_FILENAME "netdata.conf"

extern int load_config(char *filename, int overwrite_used);

extern char *config_get(const char *section, const char *name, const char *default_value);
extern long long config_get_number(const char *section, const char *name, long long value);
extern int config_get_boolean(const char *section, const char *name, int value);

extern const char *config_set(const char *section, const char *name, const char *value);
extern long long config_set_number(const char *section, const char *name, long long value);
extern int config_set_boolean(const char *section, const char *name, int value);

extern void generate_config(struct web_buffer *wb, int only_changed);

#endif /* NETDATA_CONFIG_H */
