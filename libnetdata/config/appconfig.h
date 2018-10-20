// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * This section manages ini config files, like netdata.conf and stream.conf
 *
 * It is organized like this:
 *
 * struct config (i.e. netdata.conf or stream.conf)
 *   .sections    = a linked list of struct section
 *   .mutex       = a mutex to protect the above linked list due to multi-threading
 *   .index       = an AVL tree of struct section
 *
 * struct section (i.e. [global] or [health] of netdata.conf)
 *   .value       = a linked list of struct config_option
 *   .mutex       = a mutex to protect the above linked list due to multi-threading
 *   .value_index = an AVL tree of struct config_option
 *
 * struct config_option (ie. a name-value pair for each ini file option)
 *
 * The following operations on name-value options are supported:
 *    SET           to set the value of an option
 *    SET DEFAULT   to set the value and the default value of an option
 *    GET           to get the value of an option
 *    EXISTS        to check if an option exists
 *    MOVE          to move an option from a section to another section, and/or rename it
 *
 *    GET and SET operations are provided for the following data types:
 *                  STRING
 *                  NUMBER (long long)
 *                  FLOAT (long double)
 *                  BOOLEAN (false, true)
 *                  BOOLEAN ONDEMAND (false, true, auto)
 *
 *   GET and SET operations create struct config_option, if it is not already present.
 *   This allows netdata to run even without netdata.conf and stream.conf. The internal
 *   defaults are used to create the structure that should exist in the ini file and the config
 *   file can be downloaded from the server.
 *
 *   Also 2 operations are supported for the whole config file:
 *
 *     LOAD         To load the ini file from disk
 *     GENERATE     To generate the ini file (this is used to download the ini file from the server)
 *
 * For each option (name-value pair), the system maintains 4 flags:
 *   LOADED   to indicate that the value has been loaded from the file
 *   USED     to indicate that netdata used the value
 *   CHANGED  to indicate that the value has been changed from the loaded value or the internal default value
 *   CHECKED  is used internally for optimization (to avoid an strcmp() every time GET is called).
 *
 * TODO:
 * 1. The linked lists and the mutexes can be removed and the AVL trees can become DICTIONARY.
 *    This part of the code was written before we add traversal to AVL.
 *
 * 2. High level data types could be supported, to simplify the rest of the code:
 *       MULTIPLE CHOICE  to let the user select one of the supported keywords
 *                        this would allow users see in comments the available options
 *
 *       SIMPLE PATTERN   to let the user define netdata SIMPLE PATTERNS
 *
 * 3. Sorting of options should be supported.
 *    Today, when the ini file is downloaded from the server, the options are shown in the order
 *    they appear in the linked list (the order they were added, listing changed options first).
 *    If we remove the linked list, the order they appear in the AVL tree will be used (which is
 *    random due to simple_hash()).
 *    Ideally, we support sorting of options when generating the ini file.
 *
 * 4. There is no free() operation. So, memory is freed on netdata exit.
 *
 * 5. Avoid memory fragmentation
 *    Since entries are created from multiple threads and a lot of allocations are required
 *    for each config_option, fragmentation can be a problem for IoT.
 *
 * 6. Although this way of managing options is quite flexible and dynamic, it wastes memory
 *    for the names of the options. Since most of the option names are static, we could provide
 *    a method to allocate only the dynamic option names.
 */

#ifndef NETDATA_CONFIG_H
#define NETDATA_CONFIG_H 1

#include "../libnetdata.h"

#define CONFIG_FILENAME "netdata.conf"

#define CONFIG_SECTION_GLOBAL   "global"
#define CONFIG_SECTION_WEB      "web"
#define CONFIG_SECTION_STATSD   "statsd"
#define CONFIG_SECTION_PLUGINS  "plugins"
#define CONFIG_SECTION_REGISTRY "registry"
#define CONFIG_SECTION_HEALTH   "health"
#define CONFIG_SECTION_BACKEND  "backend"
#define CONFIG_SECTION_STREAM   "stream"

// these are used to limit the configuration names and values lengths
// they are not enforced by config.c functions (they will strdup() all strings, no matter of their length)
#define CONFIG_MAX_NAME 1024
#define CONFIG_MAX_VALUE 2048

struct config {
    struct section *sections;
    netdata_mutex_t mutex;
    avl_tree_lock index;
};

#define CONFIG_BOOLEAN_NO   0
#define CONFIG_BOOLEAN_YES  1

#ifndef CONFIG_BOOLEAN_AUTO
#define CONFIG_BOOLEAN_AUTO 2
#endif

extern int appconfig_load(struct config *root, char *filename, int overwrite_used);

extern char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value);
extern long long appconfig_get_number(struct config *root, const char *section, const char *name, long long value);
extern LONG_DOUBLE appconfig_get_float(struct config *root, const char *section, const char *name, LONG_DOUBLE value);
extern int appconfig_get_boolean(struct config *root, const char *section, const char *name, int value);
extern int appconfig_get_boolean_ondemand(struct config *root, const char *section, const char *name, int value);

extern const char *appconfig_set(struct config *root, const char *section, const char *name, const char *value);
extern const char *appconfig_set_default(struct config *root, const char *section, const char *name, const char *value);
extern long long appconfig_set_number(struct config *root, const char *section, const char *name, long long value);
extern LONG_DOUBLE appconfig_set_float(struct config *root, const char *section, const char *name, LONG_DOUBLE value);
extern int appconfig_set_boolean(struct config *root, const char *section, const char *name, int value);

extern int appconfig_exists(struct config *root, const char *section, const char *name);
extern int appconfig_move(struct config *root, const char *section_old, const char *name_old, const char *section_new, const char *name_new);

extern void appconfig_generate(struct config *root, BUFFER *wb, int only_changed);

extern int appconfig_section_compare(void *a, void *b);

#endif /* NETDATA_CONFIG_H */
