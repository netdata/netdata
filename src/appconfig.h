#ifndef NETDATA_CONFIG_H
#define NETDATA_CONFIG_H 1

/**
 * @file appconfig.h
 * @brief Read and update configuration.
 *
 * Configuration options are identified by configuration section and configuration name. They contain a single value.
 *
 * Configuration options can be set from a file with load_config() or methods starting with `config_set`.
 *
 * To get options use the methods starting with `config_get`. They always allow you to set a default
 * value for the case the configuration option is was not set yet.
 */

#define CONFIG_FILENAME "netdata.conf" ///< Default configuration filename.

/// Limit the configuration option names lengths. This is not enforced by config.c functions
/// (they will strdup() all strings, no matter of their length)
#define CONFIG_MAX_NAME 1024
/// Limit the configuration option value lengths. This is not enforced by config.c functions
/// (they will strdup() all strings, no matter of their length)
#define CONFIG_MAX_VALUE 2048

/**
 * Load configuration file.
 *
 * @param filename path to file containing configuration options
 * @param overwrite_used boolean: Overwrite already used configuration names.
 * @return 1 on success. 0 on error
 */
extern int load_config(char *filename, int overwrite_used);

/**
 * Get value of a configuration option.
 *
 * If not present create the configuration option with `default_value`
 *
 * @param section of configuration option
 * @param name of configuration option
 * @param default_value to use if option was not present
 * @return configuration value
 */
extern char *config_get(const char *section, const char *name, const char *default_value);
/**
 * Get value of a configuration option.
 *
 * If not present create the configuration option with `default_value`
 *
 * @param section of configuration option
 * @param name of configuration option
 * @param default_value to use if option was not present
 * @return configuration value
 */
extern long long config_get_number(const char *section, const char *name, long long default_value);
/**
 * Get value of a configuration option.
 *
 * If not present create the configuration option with `default_value`
 *
 * @param section of configuration option
 * @param name of configuration option
 * @param default_value to use if option was not present
 * @return configuration value
 */
extern int config_get_boolean(const char *section, const char *name, int default_value);

#define CONFIG_ONDEMAND_NO 0       ///< Evaluates to no
#define CONFIG_ONDEMAND_YES 1      ///< Evaluates to yes
#define CONFIG_ONDEMAND_ONDEMAND 2 ///< Let the aplication descide if evalute to yes or no.

/**
 * Get value of a configuration option.
 *
 * If not present create the configuration option with `default_value`
 *
 * @param section of configuration option
 * @param name of configuration option
 * @param default_value to use if option was not present
 * @return configuration value
 */
extern int config_get_boolean_ondemand(const char *section, const char *name, int default_value);

/**
 * Create or update configuration option.
 *
 * Create configuration option if not present. Otherwise update configuration value.
 *
 * @param section of configuration option
 * @param name of configuration option
 * @param value New configuration value.
 * @return the new configuration value
 */
extern const char *config_set(const char *section, const char *name, const char *value);
/**
 * Create or update configuration option if not set with load_config().
 *
 * Create configuration option if not present. If configuration option was not set by load_config() update configuration option.
 *
 * @param section of configuration option
 * @param name of configuration option
 * @param value to set if not loaded with load_config()
 * @return configuration value
 */
extern const char *config_set_default(const char *section, const char *name, const char *value);
/**
 * Create or update configuration option.
 *
 * Create configuration option if not present. Otherwise update configuration value.
 *
 * @param section of configuration option
 * @param name of configuration option
 * @param value New configuration value.
 * @return the new configuration value
 */
extern long long config_set_number(const char *section, const char *name, long long value);
/**
 * Create or update configuration option.
 *
 * Create configuration option if not present. Otherwise update configuration value.
 *
 * @param section of configuration option
 * @param name of configuration option
 * @param value New configuration value.
 * @return the new configuration value
 */
extern int config_set_boolean(const char *section, const char *name, int value);

/**
 * Check if configuration option is present.
 *
 * @param section of configuration option
 * @param name of configuration option
 * @return boolean if configuration option is present
 */
extern int config_exists(const char *section, const char *name);
/**
 * Rename configuration option name.
 *
 * Rename configuration option. Do not touch configuration section.
 *
 * @param section of present configuration option
 * @param old name of present configuration option
 * @param new name of configuration option
 * @return 0 on success. -1 if configuration option is not present.
 */
extern int config_rename(const char *section, const char *old, const char *new);

/**
 * Write printable `config_file` of current set configuration options.
 * 
 * Generate a printable configuration file into buffer. Use all set configuration options.
 * 
 * @param wb Web buffer to store result in.
 * @param only_changed If true only use configuration options changed by the application.
 */
extern void generate_config(BUFFER *wb, int only_changed);

#endif /* NETDATA_CONFIG_H */
