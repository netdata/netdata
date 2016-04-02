#include "web_buffer.h"

#ifndef NETDATA_CONFIG_H
#define NETDATA_CONFIG_H 1

#define CONFIG_FILENAME "netdata.conf"

// these are used to limit the configuration names and values lengths
// they are not enforced by config.c functions (they will strdup() all strings, no matter of their length)
#define CONFIG_MAX_NAME 1024
#define CONFIG_MAX_VALUE 2048

extern int load_config(char *filename, int overwrite_used);

extern char *config_get(const char *section, const char *name, const char *default_value);
extern long long config_get_number(const char *section, const char *name, long long value);
extern int config_get_boolean(const char *section, const char *name, int value);

#define CONFIG_ONDEMAND_NO 0
#define CONFIG_ONDEMAND_YES 1
#define CONFIG_ONDEMAND_ONDEMAND 2
extern int config_get_boolean_ondemand(const char *section, const char *name, int value);

extern const char *config_set(const char *section, const char *name, const char *value);
extern const char *config_set_default(const char *section, const char *name, const char *value);
extern long long config_set_number(const char *section, const char *name, long long value);
extern int config_set_boolean(const char *section, const char *name, int value);

extern void generate_config(BUFFER *wb, int only_changed);

#endif /* NETDATA_CONFIG_H */
