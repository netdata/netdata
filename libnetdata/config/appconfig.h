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

#define CONFIG_SECTION_GLOBAL             "global"
#define CONFIG_SECTION_DIRECTORIES        "directories"
#define CONFIG_SECTION_LOGS               "logs"
#define CONFIG_SECTION_ENV_VARS           "environment variables"
#define CONFIG_SECTION_SQLITE             "sqlite"
#define CONFIG_SECTION_WEB                "web"
#define CONFIG_SECTION_WEBRTC             "webrtc"
#define CONFIG_SECTION_STATSD             "statsd"
#define CONFIG_SECTION_PLUGINS            "plugins"
#define CONFIG_SECTION_CLOUD              "cloud"
#define CONFIG_SECTION_REGISTRY           "registry"
#define CONFIG_SECTION_HEALTH             "health"
#define CONFIG_SECTION_STREAM             "stream"
#define CONFIG_SECTION_ML                 "ml"
#define CONFIG_SECTION_EXPORTING          "exporting:global"
#define CONFIG_SECTION_PROMETHEUS         "prometheus:exporter"
#define CONFIG_SECTION_HOST_LABEL         "host labels"
#define EXPORTING_CONF                    "exporting.conf"
#define CONFIG_SECTION_GLOBAL_STATISTICS  "global statistics"
#define CONFIG_SECTION_DB                 "db"


// these are used to limit the configuration names and values lengths
// they are not enforced by config.c functions (they will strdup() all strings, no matter of their length)
#define CONFIG_MAX_NAME 1024
#define CONFIG_MAX_VALUE 2048

// ----------------------------------------------------------------------------
// Config definitions
#define CONFIG_FILE_LINE_MAX ((CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 1024) * 2)

#define CONFIG_VALUE_LOADED  0x01 // has been loaded from the config
#define CONFIG_VALUE_USED    0x02 // has been accessed from the program
#define CONFIG_VALUE_CHANGED 0x04 // has been changed from the loaded value or the internal default value
#define CONFIG_VALUE_CHECKED 0x08 // has been checked if the value is different from the default

struct config_option {
    avl_t avl_node;         // the index entry of this entry - this has to be first!

    uint8_t flags;
    uint32_t hash;          // a simple hash to speed up searching
                            // we first compare hashes, and only if the hashes are equal we do string comparisons

    char *name;
    char *value;

    struct config_option *next; // config->mutex protects just this
};

struct section {
    avl_t avl_node;         // the index entry of this section - this has to be first!

    uint32_t hash;          // a simple hash to speed up searching
                            // we first compare hashes, and only if the hashes are equal we do string comparisons

    char *name;

    struct section *next;    // global config_mutex protects just this

    struct config_option *values;
    avl_tree_lock values_index;

    netdata_mutex_t mutex;  // this locks only the writers, to ensure atomic updates
                            // readers are protected using the rwlock in avl_tree_lock
};

struct config {
    struct section *first_section;
    struct section *last_section; // optimize inserting at the end
    netdata_mutex_t mutex;
    avl_tree_lock index;
};

#define CONFIG_BOOLEAN_INVALID 100  // an invalid value to check for validity (used as default initialization when needed)

#define CONFIG_BOOLEAN_NO   0       // disabled
#define CONFIG_BOOLEAN_YES  1       // enabled

#ifndef CONFIG_BOOLEAN_AUTO
#define CONFIG_BOOLEAN_AUTO 2       // enabled if it has useful info when enabled
#endif

int appconfig_load(struct config *root, char *filename, int overwrite_used, const char *section_name);
void config_section_wrlock(struct section *co);
void config_section_unlock(struct section *co);

char *appconfig_get_by_section(struct section *co, const char *name, const char *default_value);
char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value);
long long appconfig_get_number(struct config *root, const char *section, const char *name, long long value);
NETDATA_DOUBLE appconfig_get_float(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value);
int appconfig_get_boolean_by_section(struct section *co, const char *name, int value);
int appconfig_get_boolean(struct config *root, const char *section, const char *name, int value);
int appconfig_get_boolean_ondemand(struct config *root, const char *section, const char *name, int value);
int appconfig_get_duration(struct config *root, const char *section, const char *name, const char *value);

const char *appconfig_set(struct config *root, const char *section, const char *name, const char *value);
const char *appconfig_set_default(struct config *root, const char *section, const char *name, const char *value);
long long appconfig_set_number(struct config *root, const char *section, const char *name, long long value);
NETDATA_DOUBLE appconfig_set_float(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value);
int appconfig_set_boolean(struct config *root, const char *section, const char *name, int value);

int appconfig_exists(struct config *root, const char *section, const char *name);
int appconfig_move(struct config *root, const char *section_old, const char *name_old, const char *section_new, const char *name_new);

void appconfig_generate(struct config *root, BUFFER *wb, int only_changed);

int appconfig_section_compare(void *a, void *b);

void appconfig_section_destroy_non_loaded(struct config *root, const char *section);
void appconfig_section_option_destroy_non_loaded(struct config *root, const char *section, const char *name);

int config_parse_duration(const char* string, int* result);

struct section *appconfig_get_section(struct config *root, const char *name);

void appconfig_wrlock(struct config *root);
void appconfig_unlock(struct config *root);

int appconfig_test_boolean_value(char *s);

struct connector_instance {
    char instance_name[CONFIG_MAX_NAME + 1];
    char connector_name[CONFIG_MAX_NAME + 1];
};

typedef struct _connector_instance {
    struct section *connector;        // actual connector
    struct section *instance;         // This instance
    char instance_name[CONFIG_MAX_NAME + 1];
    char connector_name[CONFIG_MAX_NAME + 1];
    struct _connector_instance *next; // Next instance
} _CONNECTOR_INSTANCE;

_CONNECTOR_INSTANCE *add_connector_instance(struct section *connector, struct section *instance);

#endif /* NETDATA_CONFIG_H */
