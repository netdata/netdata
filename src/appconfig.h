#ifndef NETDATA_CONFIG_H
#define NETDATA_CONFIG_H 1

/**
 * @file appconfig.h
 * @brief This file holds the API, used to read and update configuration.
 *
 * Config options are identified by config section and config name. They contain a single value.
 *
 * Config options can be set from a file with load_config() or methods starting with `config_set`.
 *
 * To get options use the methods starting with `config_get`. They always allow you to set a default value if the config
 * option is was not set yet.
 */

#define CONFIG_FILENAME "netdata.conf" ///< default config filename

/// limit the config option names lengths. This is not enforced by config.c functions
/// (they will strdup() all strings, no matter of their length)
#define CONFIG_MAX_NAME 1024
/// limit the config option names lengths. This is not enforced by config.c functions
/// (they will strdup() all strings, no matter of their length)
#define CONFIG_MAX_VALUE 2048

/**
 * @brief Load config file
 *
 * @param filename path to file containing config options
 * @param overwrite_used boolean: overwrite already used config names
 * @return 1 on success. 0 on error.
 */
extern int load_config(char *filename, int overwrite_used);

/**
 * @brief Get value of config option
 *
 * If not present create config option with `default_value`
 *
 * @param section of config option
 * @param name of config option
 * @param default_value Create new config option with `default_value` if config option was not present
 * @return config value
 */
extern char *config_get(const char *section, const char *name, const char *default_value);
/**
 * @brief Get value of config option
 *
 * If not present create config option with `default_value`
 *
 * @param section of config option
 * @param name of config option
 * @param default_value Create new config option with `default_value` if config option was not present
 * @return config value
 */
extern long long config_get_number(const char *section, const char *name, long long default_value);
/**
 * @brief Get value of config option
 *
 * If not present create config option with `default_value`
 *
 * @param section of config option
 * @param name of config option
 * @param default_value Create new config option with `default_value` if config option was not present
 * @return config value
 */
extern int config_get_boolean(const char *section, const char *name, int default_value);

#define CONFIG_ONDEMAND_NO 0       ///< evaluates to no
#define CONFIG_ONDEMAND_YES 1      ///< evaluates to yes
#define CONFIG_ONDEMAND_ONDEMAND 2 ///< let the aplication descide if evalute to yes or no.

/**
 * @brief Get value of config option
 *
 * If not present create config option with `default_value`. Accepted values are define CONFIG_ONDEMAND_NO,
 * CONFIG_ONDEMAND_YES
 * and CONFIG_ONDEMAND_ONDEMAND.
 *
 * @param section of config option
 * @param name of config option
 * @param default_value Create new config option with `default_value` if config option was not present
 * @return config value
 */
extern int config_get_boolean_ondemand(const char *section, const char *name, int default_value);

/**
 * @brief Create or update config option
 *
 * Create config option if not present. Otherwise Update config value.
 *
 * @param section of config option
 * @param name of config option
 * @param value New config value
 * @return New config value
 */
extern const char *config_set(const char *section, const char *name, const char *value);
/**
 * @brief Create or update config option if not set with load_config()
 *
 * Create config option if not present. If config option was not set by load_config() update config option.
 *
 * @param section of config option
 * @param name of config option
 * @param value config value to set if not loaded wit load_config()
 * @return config value
 */
extern const char *config_set_default(const char *section, const char *name, const char *value);
/**
 * @brief Create or update config option
 *
 * Create config option if not present. Otherwise Update config value.
 *
 * @param section of config option
 * @param name of config option
 * @param value New config value
 * @return New config value
 */
extern long long config_set_number(const char *section, const char *name, long long value);
/**
 * @brief Create or update config option
 *
 * Create config option if not present. Otherwise Update config value.
 *
 * @param section of config option
 * @param name of config option
 * @param value New config value
 * @return New config value
 */
extern int config_set_boolean(const char *section, const char *name, int value);

/**
 * @brief Check if config option is present
 *
 * @param section of config option
 * @param name of config option
 * @return boolean if config option is present
 */
extern int config_exists(const char *section, const char *name);
/**
 * @brief Rename config option name
 *
 * Rename config option. Do not touch config section.
 *
 * @param section of present config option
 * @param old name of present config option
 * @param new name of config option
 * @return 0 on success. -1 if config option is not present.
 */
extern int config_rename(const char *section, const char *old, const char *new);

/**
 * @brief Write printable config_file of current set config options.
 * 
 * Generate a printable config file into buffer. Use all set config options.
 * 
 * @param wb BUFFER to write the content info
 * @param only_changed if true only use config options changed by the application
 */
extern void generate_config(BUFFER *wb, int only_changed);

#endif /* NETDATA_CONFIG_H */
