// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  Based on:
 *  ipmimonitoring-sensors.c,v 1.51 2016/11/02 23:46:24 chu11 Exp
 *  ipmimonitoring-sel.c,v 1.51 2016/11/02 23:46:24 chu11 Exp
 *
 *  Copyright (C) 2007-2015 Lawrence Livermore National Security, LLC.
 *  Copyright (C) 2006-2007 The Regents of the University of California.
 *  Produced at Lawrence Livermore National Laboratory (cf, DISCLAIMER).
 *  Written by Albert Chu <chu11@llnl.gov>
 *  UCRL-CODE-222073
 */

// ----------------------------------------------------------------------------
// BEGIN NETDATA CODE

// #define NETDATA_TIMING_REPORT 1
#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#define FREEIPMI_GLOBAL_FUNCTION_SENSORS() do { \
        fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"ipmi-sensors\" %d \"%s\" \"top\" "HTTP_ACCESS_FORMAT" %d\n", \
                5, "Displays current sensor state and readings",                                                     \
                (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_NONE), 100); \
    } while(0)

// component names, based on our patterns
#define NETDATA_SENSOR_COMPONENT_MEMORY_MODULE     "Memory Module"
#define NETDATA_SENSOR_COMPONENT_MEMORY            "Memory"
#define NETDATA_SENSOR_COMPONENT_PROCESSOR         "Processor"
#define NETDATA_SENSOR_COMPONENT_IPU               "Image Processor"
#define NETDATA_SENSOR_COMPONENT_STORAGE           "Storage"
#define NETDATA_SENSOR_COMPONENT_MOTHERBOARD       "Motherboard"
#define NETDATA_SENSOR_COMPONENT_NETWORK           "Network"
#define NETDATA_SENSOR_COMPONENT_POWER_SUPPLY      "Power Supply"
#define NETDATA_SENSOR_COMPONENT_SYSTEM            "System"
#define NETDATA_SENSOR_COMPONENT_PERIPHERAL        "Peripheral"

// netdata plugin defaults
#define SENSORS_DICT_KEY_SIZE 2048                  // the max size of the key for the dictionary of sensors
#define SPEED_TEST_ITERATIONS 5                     // how many times to repeat data collection to decide latency
#define IPMI_SENSORS_DASHBOARD_PRIORITY 90000       // the priority of the sensors charts on the dashboard
#define IPMI_SEL_DASHBOARD_PRIORITY 99000           // the priority of the SEL events chart on the dashboard
#define IPMI_SENSORS_MIN_UPDATE_EVERY 5             // the minimum data collection frequency for sensors
#define IPMI_SEL_MIN_UPDATE_EVERY 30                // the minimum data collection frequency for SEL events
#define IPMI_ENABLE_SEL_BY_DEFAULT true             // true/false, to enable/disable SEL by default
#define IPMI_RESTART_EVERY_SECONDS 14400            // restart the plugin every this many seconds
                                                    // this is to prevent possible bugs/leaks in ipmi libraries
#define IPMI_RESTART_IF_SENSORS_DONT_ITERATE_EVERY_SECONDS (10 * 60) // stale data collection detection time

// forward definition of functions and structures
struct netdata_ipmi_state;
static void netdata_update_ipmi_sensor_reading(
        int record_id
        , int sensor_number
        , int sensor_type
        , int sensor_state
        , int sensor_units
        , int sensor_reading_type
        , char *sensor_name
        , void *sensor_reading
        , int event_reading_type_code
        , int sensor_bitmask_type
        , int sensor_bitmask
        , char **sensor_bitmask_strings
        , struct netdata_ipmi_state *stt);
static void netdata_update_ipmi_sel_events_count(struct netdata_ipmi_state *stt, uint32_t events);

// END NETDATA CODE
// ----------------------------------------------------------------------------

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <assert.h>
#include <errno.h>
#include <unistd.h>
#include <sys/time.h>

#include <ipmi_monitoring.h>
#include <ipmi_monitoring_bitmasks.h>
#include <ipmi_monitoring_offsets.h>

/* Communication Configuration - Initialize accordingly */

static netdata_mutex_t stdout_mutex = NETDATA_MUTEX_INITIALIZER;
static bool function_plugin_should_exit = false;

int update_every = IPMI_SENSORS_MIN_UPDATE_EVERY; // this is the minimum update frequency
int update_every_sel = IPMI_SEL_MIN_UPDATE_EVERY; // this is the minimum update frequency for SEL events

/* Hostname, NULL for In-band communication, non-null for a hostname */
char *hostname = NULL;

/* In-band Communication Configuration */
int driver_type = -1;               // IPMI_MONITORING_DRIVER_TYPE_KCS, etc. or -1 for default
int disable_auto_probe = 0;         /* probe for in-band device */
unsigned int driver_address = 0;    /* not used if probing */
unsigned int register_spacing = 0;  /* not used if probing */
char *driver_device = NULL;         /* not used if probing */

/* Out-of-band Communication Configuration */
int freeimpi_protocol_version = -1;      // IPMI_MONITORING_PROTOCOL_VERSION_1_5, etc. or -1 for default
char *username = "";
char *password = "";
unsigned char *k_g = NULL;
unsigned int k_g_len = 0;
int privilege_level = -1;       // IPMI_MONITORING_PRIVILEGE_LEVEL_USER, etc. or -1 for default
int authentication_type = -1;   // IPMI_MONITORING_AUTHENTICATION_TYPE_MD5, etc. or -1 for default
int cipher_suite_id = -1;       /* 0 or -1 for default */
int session_timeout = 0;        /* 0 for default */
int retransmission_timeout = 0; /* 0 for default */

/* Workarounds - specify workaround flags if necessary */
unsigned int workaround_flags = 0;

/* Set to an appropriate alternate if desired */
char *sdr_cache_directory = "/tmp";
char *sdr_sensors_cache_format = ".netdata-freeipmi-sensors-%H-on-%L.sdr";
char *sdr_sel_cache_format = ".netdata-freeipmi-sel-%H-on-%L.sdr";
char *sensor_config_file = NULL;
char *sel_config_file = NULL;

// controlled via command line options
unsigned int global_sel_flags = IPMI_MONITORING_SEL_FLAGS_REREAD_SDR_CACHE;
unsigned int global_sensor_reading_flags = IPMI_MONITORING_SENSOR_READING_FLAGS_DISCRETE_READING|IPMI_MONITORING_SENSOR_READING_FLAGS_REREAD_SDR_CACHE;
bool remove_reread_sdr_after_first_use = true;

/* Initialization flags
 *
 * Most commonly bitwise OR IPMI_MONITORING_FLAGS_DEBUG and/or
 * IPMI_MONITORING_FLAGS_DEBUG_IPMI_PACKETS for extra debugging
 * information.
 */
unsigned int ipmimonitoring_init_flags = 0;

// ----------------------------------------------------------------------------
// functions common to sensors and SEL

static void initialize_ipmi_config (struct ipmi_monitoring_ipmi_config *ipmi_config) {
    fatal_assert(ipmi_config);

    ipmi_config->driver_type = driver_type;
    ipmi_config->disable_auto_probe = disable_auto_probe;
    ipmi_config->driver_address = driver_address;
    ipmi_config->register_spacing = register_spacing;
    ipmi_config->driver_device = driver_device;

    ipmi_config->protocol_version = freeimpi_protocol_version;
    ipmi_config->username = username;
    ipmi_config->password = password;
    ipmi_config->k_g = k_g;
    ipmi_config->k_g_len = k_g_len;
    ipmi_config->privilege_level = privilege_level;
    ipmi_config->authentication_type = authentication_type;
    ipmi_config->cipher_suite_id = cipher_suite_id;
    ipmi_config->session_timeout_len = session_timeout;
    ipmi_config->retransmission_timeout_len = retransmission_timeout;

    ipmi_config->workaround_flags = workaround_flags;
}

static const char *netdata_ipmi_get_sensor_type_string (int sensor_type, const char **component) {
    switch (sensor_type) {
        case IPMI_MONITORING_SENSOR_TYPE_RESERVED:
            return ("Reserved");

        case IPMI_MONITORING_SENSOR_TYPE_TEMPERATURE:
            return ("Temperature");

        case IPMI_MONITORING_SENSOR_TYPE_VOLTAGE:
            return ("Voltage");

        case IPMI_MONITORING_SENSOR_TYPE_CURRENT:
            return ("Current");

        case IPMI_MONITORING_SENSOR_TYPE_FAN:
            return ("Fan");

        case IPMI_MONITORING_SENSOR_TYPE_PHYSICAL_SECURITY:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Physical Security");

        case IPMI_MONITORING_SENSOR_TYPE_PLATFORM_SECURITY_VIOLATION_ATTEMPT:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Platform Security Violation Attempt");

        case IPMI_MONITORING_SENSOR_TYPE_PROCESSOR:
            *component = NETDATA_SENSOR_COMPONENT_PROCESSOR;
            return ("Processor");

        case IPMI_MONITORING_SENSOR_TYPE_POWER_SUPPLY:
            *component = NETDATA_SENSOR_COMPONENT_POWER_SUPPLY;
            return ("Power Supply");

        case IPMI_MONITORING_SENSOR_TYPE_POWER_UNIT:
            *component = NETDATA_SENSOR_COMPONENT_POWER_SUPPLY;
            return ("Power Unit");

        case IPMI_MONITORING_SENSOR_TYPE_COOLING_DEVICE:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Cooling Device");

        case IPMI_MONITORING_SENSOR_TYPE_OTHER_UNITS_BASED_SENSOR:
            return ("Other Units Based Sensor");

        case IPMI_MONITORING_SENSOR_TYPE_MEMORY:
            *component = NETDATA_SENSOR_COMPONENT_MEMORY;
            return ("Memory");

        case IPMI_MONITORING_SENSOR_TYPE_DRIVE_SLOT:
            *component = NETDATA_SENSOR_COMPONENT_STORAGE;
            return ("Drive Slot");

        case IPMI_MONITORING_SENSOR_TYPE_POST_MEMORY_RESIZE:
            *component = NETDATA_SENSOR_COMPONENT_MEMORY;
            return ("POST Memory Resize");

        case IPMI_MONITORING_SENSOR_TYPE_SYSTEM_FIRMWARE_PROGRESS:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("System Firmware Progress");

        case IPMI_MONITORING_SENSOR_TYPE_EVENT_LOGGING_DISABLED:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Event Logging Disabled");

        case IPMI_MONITORING_SENSOR_TYPE_WATCHDOG1:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Watchdog 1");

        case IPMI_MONITORING_SENSOR_TYPE_SYSTEM_EVENT:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("System Event");

        case IPMI_MONITORING_SENSOR_TYPE_CRITICAL_INTERRUPT:
            return ("Critical Interrupt");

        case IPMI_MONITORING_SENSOR_TYPE_BUTTON_SWITCH:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Button/Switch");

        case IPMI_MONITORING_SENSOR_TYPE_MODULE_BOARD:
            return ("Module/Board");

        case IPMI_MONITORING_SENSOR_TYPE_MICROCONTROLLER_COPROCESSOR:
            *component = NETDATA_SENSOR_COMPONENT_PROCESSOR;
            return ("Microcontroller/Coprocessor");

        case IPMI_MONITORING_SENSOR_TYPE_ADD_IN_CARD:
            return ("Add In Card");

        case IPMI_MONITORING_SENSOR_TYPE_CHASSIS:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Chassis");

        case IPMI_MONITORING_SENSOR_TYPE_CHIP_SET:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Chip Set");

        case IPMI_MONITORING_SENSOR_TYPE_OTHER_FRU:
            return ("Other Fru");

        case IPMI_MONITORING_SENSOR_TYPE_CABLE_INTERCONNECT:
            return ("Cable/Interconnect");

        case IPMI_MONITORING_SENSOR_TYPE_TERMINATOR:
            return ("Terminator");

        case IPMI_MONITORING_SENSOR_TYPE_SYSTEM_BOOT_INITIATED:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("System Boot Initiated");

        case IPMI_MONITORING_SENSOR_TYPE_BOOT_ERROR:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Boot Error");

        case IPMI_MONITORING_SENSOR_TYPE_OS_BOOT:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("OS Boot");

        case IPMI_MONITORING_SENSOR_TYPE_OS_CRITICAL_STOP:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("OS Critical Stop");

        case IPMI_MONITORING_SENSOR_TYPE_SLOT_CONNECTOR:
            return ("Slot/Connector");

        case IPMI_MONITORING_SENSOR_TYPE_SYSTEM_ACPI_POWER_STATE:
            return ("System ACPI Power State");

        case IPMI_MONITORING_SENSOR_TYPE_WATCHDOG2:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Watchdog 2");

        case IPMI_MONITORING_SENSOR_TYPE_PLATFORM_ALERT:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Platform Alert");

        case IPMI_MONITORING_SENSOR_TYPE_ENTITY_PRESENCE:
            return ("Entity Presence");

        case IPMI_MONITORING_SENSOR_TYPE_MONITOR_ASIC_IC:
            return ("Monitor ASIC/IC");

        case IPMI_MONITORING_SENSOR_TYPE_LAN:
            *component = NETDATA_SENSOR_COMPONENT_NETWORK;
            return ("LAN");

        case IPMI_MONITORING_SENSOR_TYPE_MANAGEMENT_SUBSYSTEM_HEALTH:
            *component = NETDATA_SENSOR_COMPONENT_SYSTEM;
            return ("Management Subsystem Health");

        case IPMI_MONITORING_SENSOR_TYPE_BATTERY:
            return ("Battery");

        case IPMI_MONITORING_SENSOR_TYPE_SESSION_AUDIT:
            return ("Session Audit");

        case IPMI_MONITORING_SENSOR_TYPE_VERSION_CHANGE:
            return ("Version Change");

        case IPMI_MONITORING_SENSOR_TYPE_FRU_STATE:
            return ("FRU State");

        case IPMI_MONITORING_SENSOR_TYPE_UNKNOWN:
            return ("Unknown");

        default:
            if(sensor_type >= IPMI_MONITORING_SENSOR_TYPE_OEM_MIN && sensor_type <= IPMI_MONITORING_SENSOR_TYPE_OEM_MAX)
                return ("OEM");

            return ("Unrecognized");
    }
}

#define netdata_ipmi_get_value_int(var, func, ctx) do {         \
    (var) = func(ctx);                                          \
    if( (var) < 0) {                                            \
        collector_error("%s(): call to " #func " failed: %s",   \
            __FUNCTION__, ipmi_monitoring_ctx_errormsg(ctx));   \
        goto cleanup;                                           \
    }                                                           \
    timing_step(TIMING_STEP_FREEIPMI_READ_ ## var);             \
} while(0)

#define netdata_ipmi_get_value_ptr(var, func, ctx) do {         \
    (var) = func(ctx);                                          \
    if(!(var)) {                                                \
        collector_error("%s(): call to " #func " failed: %s",   \
            __FUNCTION__, ipmi_monitoring_ctx_errormsg(ctx));   \
        goto cleanup;                                           \
    }                                                           \
    timing_step(TIMING_STEP_FREEIPMI_READ_ ## var);             \
} while(0)

#define netdata_ipmi_get_value_no_check(var, func, ctx) do {    \
    (var) = func(ctx);                                          \
    timing_step(TIMING_STEP_FREEIPMI_READ_ ## var);             \
} while(0)

static int netdata_read_ipmi_sensors(struct ipmi_monitoring_ipmi_config *ipmi_config, struct netdata_ipmi_state *state) {
    timing_init();

    ipmi_monitoring_ctx_t ctx = NULL;
    unsigned int sensor_reading_flags = global_sensor_reading_flags;
    int i;
    int sensor_count;
    int rv = -1;

    if (!(ctx = ipmi_monitoring_ctx_create ())) {
        collector_error("ipmi_monitoring_ctx_create()");
        goto cleanup;
    }

    timing_step(TIMING_STEP_FREEIPMI_CTX_CREATE);

    if (sdr_cache_directory) {
        if (ipmi_monitoring_ctx_sdr_cache_directory (ctx, sdr_cache_directory) < 0) {
            collector_error("ipmi_monitoring_ctx_sdr_cache_directory(): %s\n", ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    if (sdr_sensors_cache_format) {
        if (ipmi_monitoring_ctx_sdr_cache_filenames(ctx, sdr_sensors_cache_format) < 0) {
            collector_error("ipmi_monitoring_ctx_sdr_cache_filenames(): %s\n", ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

    timing_step(TIMING_STEP_FREEIPMI_DSR_CACHE_DIR);

    // Must call otherwise only default interpretations ever used
    // sensor_config_file can be NULL
    if (ipmi_monitoring_ctx_sensor_config_file (ctx, sensor_config_file) < 0) {
        collector_error( "ipmi_monitoring_ctx_sensor_config_file(): %s\n", ipmi_monitoring_ctx_errormsg (ctx));
        goto cleanup;
    }

    timing_step(TIMING_STEP_FREEIPMI_SENSOR_CONFIG_FILE);

    if ((sensor_count = ipmi_monitoring_sensor_readings_by_record_id (ctx,
            hostname,
            ipmi_config,
            sensor_reading_flags,
            NULL,
            0,
            NULL,
            NULL)) < 0) {
        collector_error( "ipmi_monitoring_sensor_readings_by_record_id(): %s",
                         ipmi_monitoring_ctx_errormsg (ctx));
        goto cleanup;
    }

    timing_step(TIMING_STEP_FREEIPMI_SENSOR_READINGS_BY_X);

    for (i = 0; i < sensor_count; i++, ipmi_monitoring_sensor_iterator_next (ctx)) {
        int record_id, sensor_number, sensor_type, sensor_state, sensor_units,
            sensor_bitmask_type, sensor_bitmask, event_reading_type_code, sensor_reading_type;

        char **sensor_bitmask_strings = NULL;
        char *sensor_name = NULL;
        void *sensor_reading;

        netdata_ipmi_get_value_int(record_id, ipmi_monitoring_sensor_read_record_id, ctx);
        netdata_ipmi_get_value_int(sensor_number, ipmi_monitoring_sensor_read_sensor_number, ctx);
        netdata_ipmi_get_value_int(sensor_type, ipmi_monitoring_sensor_read_sensor_type, ctx);
        netdata_ipmi_get_value_ptr(sensor_name, ipmi_monitoring_sensor_read_sensor_name, ctx);
        netdata_ipmi_get_value_int(sensor_state, ipmi_monitoring_sensor_read_sensor_state, ctx);
        netdata_ipmi_get_value_int(sensor_units, ipmi_monitoring_sensor_read_sensor_units, ctx);
        netdata_ipmi_get_value_int(sensor_bitmask_type, ipmi_monitoring_sensor_read_sensor_bitmask_type, ctx);
        netdata_ipmi_get_value_int(sensor_bitmask, ipmi_monitoring_sensor_read_sensor_bitmask, ctx);
        // it's ok for this to be NULL, i.e. sensor_bitmask == IPMI_MONITORING_SENSOR_BITMASK_TYPE_UNKNOWN
        netdata_ipmi_get_value_no_check(sensor_bitmask_strings, ipmi_monitoring_sensor_read_sensor_bitmask_strings, ctx);
        netdata_ipmi_get_value_int(sensor_reading_type, ipmi_monitoring_sensor_read_sensor_reading_type, ctx);
        // whatever we read from the sensor, it is ok
        netdata_ipmi_get_value_no_check(sensor_reading, ipmi_monitoring_sensor_read_sensor_reading, ctx);
        netdata_ipmi_get_value_int(event_reading_type_code, ipmi_monitoring_sensor_read_event_reading_type_code, ctx);

        netdata_update_ipmi_sensor_reading(
                record_id, sensor_number, sensor_type, sensor_state, sensor_units, sensor_reading_type, sensor_name,
                sensor_reading, event_reading_type_code, sensor_bitmask_type, sensor_bitmask, sensor_bitmask_strings,
                state
        );

#ifdef NETDATA_COMMENTED
        /* It is possible you may want to monitor specific event
         * conditions that may occur.  If that is the case, you may want
         * to check out what specific bitmask type and bitmask events
         * occurred.  See ipmi_monitoring_bitmasks.h for a list of
         * bitmasks and types.
         */

        if (sensor_bitmask_type != IPMI_MONITORING_SENSOR_BITMASK_TYPE_UNKNOWN)
            printf (", %Xh", sensor_bitmask);
        else
            printf (", N/A");

        if (sensor_bitmask_type != IPMI_MONITORING_SENSOR_BITMASK_TYPE_UNKNOWN
            && sensor_bitmask_strings)
        {
            unsigned int i = 0;

            printf (",");

            while (sensor_bitmask_strings[i])
            {
                printf (" ");

                printf ("'%s'",
                        sensor_bitmask_strings[i]);

                i++;
            }
        }
        else
            printf (", N/A");

        printf ("\n");
#endif // NETDATA_COMMENTED
    }

    rv = 0;

cleanup:
    if (ctx)
        ipmi_monitoring_ctx_destroy (ctx);

    timing_report();

    if(remove_reread_sdr_after_first_use)
        global_sensor_reading_flags &= ~(IPMI_MONITORING_SENSOR_READING_FLAGS_REREAD_SDR_CACHE);

    return (rv);
}


static int netdata_get_ipmi_sel_events_count(struct ipmi_monitoring_ipmi_config *ipmi_config, struct netdata_ipmi_state *state) {
    timing_init();

    ipmi_monitoring_ctx_t ctx = NULL;
    unsigned int sel_flags = global_sel_flags;
    int sel_count;
    int rv = -1;

    if (!(ctx = ipmi_monitoring_ctx_create ())) {
        collector_error("ipmi_monitoring_ctx_create()");
        goto cleanup;
    }

    if (sdr_cache_directory) {
        if (ipmi_monitoring_ctx_sdr_cache_directory (ctx, sdr_cache_directory) < 0) {
            collector_error( "ipmi_monitoring_ctx_sdr_cache_directory(): %s", ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    if (sdr_sel_cache_format) {
        if (ipmi_monitoring_ctx_sdr_cache_filenames(ctx, sdr_sel_cache_format) < 0) {
            collector_error("ipmi_monitoring_ctx_sdr_cache_filenames(): %s\n", ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

    // Must call otherwise only default interpretations ever used
    // sel_config_file can be NULL
    if (ipmi_monitoring_ctx_sel_config_file (ctx, sel_config_file) < 0) {
        collector_error( "ipmi_monitoring_ctx_sel_config_file(): %s",
                         ipmi_monitoring_ctx_errormsg (ctx));
        goto cleanup;
    }

    if ((sel_count = ipmi_monitoring_sel_by_record_id (ctx,
            hostname,
            ipmi_config,
            sel_flags,
            NULL,
            0,
            NULL,
            NULL)) < 0) {
        collector_error( "ipmi_monitoring_sel_by_record_id(): %s",
                         ipmi_monitoring_ctx_errormsg (ctx));
        goto cleanup;
    }

    netdata_update_ipmi_sel_events_count(state, sel_count);

    rv = 0;

cleanup:
    if (ctx)
        ipmi_monitoring_ctx_destroy (ctx);

    timing_report();

    if(remove_reread_sdr_after_first_use)
        global_sel_flags &= ~(IPMI_MONITORING_SEL_FLAGS_REREAD_SDR_CACHE);

    return (rv);
}

// ----------------------------------------------------------------------------
// copied from freeipmi codebase commit 8dea6dec4012d0899901e595f2c868a05e1cefed
// added netdata_ in-front to not overwrite library functions

// FROM: common/miscutil/network.c
static int netdata_host_is_localhost (const char *host) {
    /* Ordered by my assumption of most popular */
    if (!strcasecmp (host, "localhost")
        || !strcmp (host, "127.0.0.1")
        || !strcasecmp (host, "ipv6-localhost")
        || !strcmp (host, "::1")
        || !strcasecmp (host, "ip6-localhost")
        || !strcmp (host, "0:0:0:0:0:0:0:1"))
        return (1);

    return (0);
}

// FROM: common/parsecommon/parse-common.h
#define IPMI_PARSE_DEVICE_LAN_STR       "lan"
#define IPMI_PARSE_DEVICE_LAN_2_0_STR   "lan_2_0"
#define IPMI_PARSE_DEVICE_LAN_2_0_STR2  "lan20"
#define IPMI_PARSE_DEVICE_LAN_2_0_STR3  "lan_20"
#define IPMI_PARSE_DEVICE_LAN_2_0_STR4  "lan2_0"
#define IPMI_PARSE_DEVICE_LAN_2_0_STR5  "lanplus"
#define IPMI_PARSE_DEVICE_KCS_STR       "kcs"
#define IPMI_PARSE_DEVICE_SSIF_STR      "ssif"
#define IPMI_PARSE_DEVICE_OPENIPMI_STR  "openipmi"
#define IPMI_PARSE_DEVICE_OPENIPMI_STR2 "open"
#define IPMI_PARSE_DEVICE_SUNBMC_STR    "sunbmc"
#define IPMI_PARSE_DEVICE_SUNBMC_STR2   "bmc"
#define IPMI_PARSE_DEVICE_INTELDCMI_STR "inteldcmi"

// FROM: common/parsecommon/parse-common.c
// changed the return values to match ipmi_monitoring.h
static int netdata_parse_outofband_driver_type (const char *str) {
    if (strcasecmp (str, IPMI_PARSE_DEVICE_LAN_STR) == 0)
        return (IPMI_MONITORING_PROTOCOL_VERSION_1_5);

        /* support "lanplus" for those that might be used to ipmitool.
         * support typo variants to ease.
         */
    else if (strcasecmp (str, IPMI_PARSE_DEVICE_LAN_2_0_STR) == 0
             || strcasecmp (str, IPMI_PARSE_DEVICE_LAN_2_0_STR2) == 0
             || strcasecmp (str, IPMI_PARSE_DEVICE_LAN_2_0_STR3) == 0
             || strcasecmp (str, IPMI_PARSE_DEVICE_LAN_2_0_STR4) == 0
             || strcasecmp (str, IPMI_PARSE_DEVICE_LAN_2_0_STR5) == 0)
        return (IPMI_MONITORING_PROTOCOL_VERSION_2_0);

    return (-1);
}

// FROM: common/parsecommon/parse-common.c
// changed the return values to match ipmi_monitoring.h
static int netdata_parse_inband_driver_type (const char *str) {
    if (strcasecmp (str, IPMI_PARSE_DEVICE_KCS_STR) == 0)
        return (IPMI_MONITORING_DRIVER_TYPE_KCS);
    else if (strcasecmp (str, IPMI_PARSE_DEVICE_SSIF_STR) == 0)
        return (IPMI_MONITORING_DRIVER_TYPE_SSIF);
        /* support "open" for those that might be used to
         * ipmitool.
         */
    else if (strcasecmp (str, IPMI_PARSE_DEVICE_OPENIPMI_STR) == 0
             || strcasecmp (str, IPMI_PARSE_DEVICE_OPENIPMI_STR2) == 0)
        return (IPMI_MONITORING_DRIVER_TYPE_OPENIPMI);
        /* support "bmc" for those that might be used to
         * ipmitool.
         */
    else if (strcasecmp (str, IPMI_PARSE_DEVICE_SUNBMC_STR) == 0
             || strcasecmp (str, IPMI_PARSE_DEVICE_SUNBMC_STR2) == 0)
        return (IPMI_MONITORING_DRIVER_TYPE_SUNBMC);

#ifdef IPMI_MONITORING_DRIVER_TYPE_INTELDCMI
    else if (strcasecmp (str, IPMI_PARSE_DEVICE_INTELDCMI_STR) == 0)
        return (IPMI_MONITORING_DRIVER_TYPE_INTELDCMI);
#endif // IPMI_MONITORING_DRIVER_TYPE_INTELDCMI

    return (-1);
}

// ----------------------------------------------------------------------------
// BEGIN NETDATA CODE

typedef enum __attribute__((packed)) {
    IPMI_COLLECT_TYPE_SENSORS = (1 << 0),
    IPMI_COLLECT_TYPE_SEL     = (1 << 1),
} IPMI_COLLECTION_TYPE;

struct sensor {
    int sensor_type;
    int sensor_state;
    int sensor_units;
    char *sensor_name;

    int sensor_reading_type;
    union {
        uint8_t bool_value;
        uint32_t uint32_value;
        double double_value;
    } sensor_reading;

    // netdata provided
    const char *context;
    const char *title;
    const char *units;
    const char *family;
    const char *chart_type;
    const char *dimension;
    int priority;

    const char *type;
    const char *component;

    int multiplier;
    bool do_metric;
    bool do_state;
    bool metric_chart_sent;
    bool state_chart_sent;
    usec_t last_collected_metric_ut;
    usec_t last_collected_state_ut;
};

typedef enum __attribute__((packed)) {
    ICS_INIT,
    ICS_INIT_FAILED,
    ICS_RUNNING,
    ICS_FAILED,
} IPMI_COLLECTOR_STATUS;

struct netdata_ipmi_state {
    bool debug;

    struct {
        IPMI_COLLECTOR_STATUS status;
        usec_t last_iteration_ut;
        size_t collected;
        usec_t now_ut;
        usec_t freq_ut;
        int priority;
        DICTIONARY *dict;
    } sensors;

    struct {
        IPMI_COLLECTOR_STATUS status;
        usec_t last_iteration_ut;
        size_t events;
        usec_t now_ut;
        usec_t freq_ut;
        int priority;
    } sel;

    struct {
        usec_t now_ut;
    } updates;
};

struct netdata_ipmi_state state = {0};

// ----------------------------------------------------------------------------
// excluded record ids maintenance (both for sensor data and state)

static int *excluded_record_ids = NULL;
size_t excluded_record_ids_length = 0;

static void excluded_record_ids_parse(const char *s, bool debug) {
    if(!s) return;

    while(*s) {
        while(*s && !isdigit(*s)) s++;

        if(isdigit(*s)) {
            char *e;
            unsigned long n = strtoul(s, &e, 10);
            s = e;

            if(n != 0) {
                excluded_record_ids = reallocz(excluded_record_ids, (excluded_record_ids_length + 1) * sizeof(int));
                excluded_record_ids[excluded_record_ids_length++] = (int)n;
            }
        }
    }

    if(debug) {
        fprintf(stderr, "%s: excluded record ids:", program_name);
        size_t i;
        for(i = 0; i < excluded_record_ids_length; i++) {
            fprintf(stderr, " %d", excluded_record_ids[i]);
        }
        fprintf(stderr, "\n");
    }
}

static int *excluded_status_record_ids = NULL;
size_t excluded_status_record_ids_length = 0;

static void excluded_status_record_ids_parse(const char *s, bool debug) {
    if(!s) return;

    while(*s) {
        while(*s && !isdigit(*s)) s++;

        if(isdigit(*s)) {
            char *e;
            unsigned long n = strtoul(s, &e, 10);
            s = e;

            if(n != 0) {
                excluded_status_record_ids = reallocz(excluded_status_record_ids, (excluded_status_record_ids_length + 1) * sizeof(int));
                excluded_status_record_ids[excluded_status_record_ids_length++] = (int)n;
            }
        }
    }

    if(debug) {
        fprintf(stderr, "%s: excluded status record ids:", program_name);
        size_t i;
        for(i = 0; i < excluded_status_record_ids_length; i++) {
            fprintf(stderr, " %d", excluded_status_record_ids[i]);
        }
        fprintf(stderr, "\n");
    }
}


static int excluded_record_ids_check(int record_id) {
    size_t i;

    for(i = 0; i < excluded_record_ids_length; i++) {
        if(excluded_record_ids[i] == record_id)
            return 1;
    }

    return 0;
}

static int excluded_status_record_ids_check(int record_id) {
    size_t i;

    for(i = 0; i < excluded_status_record_ids_length; i++) {
        if(excluded_status_record_ids[i] == record_id)
            return 1;
    }

    return 0;
}

// ----------------------------------------------------------------------------
// data collection functions

struct {
    const char *search;
    SIMPLE_PATTERN *pattern;
    const char *label;
} sensors_component_patterns[] = {

        // The order is important!
        // They are evaluated top to bottom
        // The first the matches is used

        {
                .search = "*DIMM*|*_DIM*|*VTT*|*VDDQ*|*ECC*|*MEM*CRC*|*MEM*BD*",
                .label = NETDATA_SENSOR_COMPONENT_MEMORY_MODULE,
        },
        {
                .search = "*CPU*|SOC_*|*VDDCR*|P*_VDD*|*_DTS|*VCORE*|*PROC*",
                .label = NETDATA_SENSOR_COMPONENT_PROCESSOR,
        },
        {
                .search = "IPU*",
                .label = NETDATA_SENSOR_COMPONENT_IPU,
        },
        {
                .search = "M2_*|*SSD*|*HSC*|*HDD*|*NVME*",
                .label = NETDATA_SENSOR_COMPONENT_STORAGE,
        },
        {
                .search = "MB_*|*PCH*|*VBAT*|*I/O*BD*|*IO*BD*",
                .label = NETDATA_SENSOR_COMPONENT_MOTHERBOARD,
        },
        {
                .search = "Watchdog|SEL|SYS_*|*CHASSIS*",
                .label = NETDATA_SENSOR_COMPONENT_SYSTEM,
        },
        {
                .search = "PS*|P_*|*PSU*|*PWR*|*TERMV*|*D2D*",
                .label = NETDATA_SENSOR_COMPONENT_POWER_SUPPLY,
        },

        // fallback components
        {
                .search = "VR_P*|*VRMP*",
                .label = NETDATA_SENSOR_COMPONENT_PROCESSOR,
        },
        {
                .search = "*VSB*|*PS*",
                .label = NETDATA_SENSOR_COMPONENT_POWER_SUPPLY,
        },
        {
                .search = "*MEM*|*MEM*RAID*",
                .label = NETDATA_SENSOR_COMPONENT_MEMORY,
        },
        {
                .search = "*RAID*",         // there is also "Memory RAID", so keep this after memory
                .label = NETDATA_SENSOR_COMPONENT_STORAGE,
        },
        {
                .search = "*PERIPHERAL*|*USB*",
                .label = NETDATA_SENSOR_COMPONENT_PERIPHERAL,
        },
        {
                .search = "*FAN*|*12V*|*VCC*|*PCI*|*CHIPSET*|*AMP*|*BD*",
                .label = NETDATA_SENSOR_COMPONENT_SYSTEM,
        },

        // terminator
        {
                .search = NULL,
                .label = NULL,
        }
};

static const char *netdata_sensor_name_to_component(const char *sensor_name) {
    for(int i = 0; sensors_component_patterns[i].search ;i++) {
        if(!sensors_component_patterns[i].pattern)
            sensors_component_patterns[i].pattern = simple_pattern_create(sensors_component_patterns[i].search, "|", SIMPLE_PATTERN_EXACT, false);

        if(simple_pattern_matches(sensors_component_patterns[i].pattern, sensor_name))
            return sensors_component_patterns[i].label;
    }

    return "Other";
}

const char *netdata_collect_type_to_string(IPMI_COLLECTION_TYPE type) {
    if((type & (IPMI_COLLECT_TYPE_SENSORS|IPMI_COLLECT_TYPE_SEL)) == (IPMI_COLLECT_TYPE_SENSORS|IPMI_COLLECT_TYPE_SEL))
        return "sensors,sel";
    if(type & IPMI_COLLECT_TYPE_SEL)
        return "sel";
    if(type & IPMI_COLLECT_TYPE_SENSORS)
        return "sensors";

    return "unknown";
}

static void netdata_sensor_set_value(struct sensor *sn, void *sensor_reading, struct netdata_ipmi_state *stt __maybe_unused) {
    switch(sn->sensor_reading_type) {
        case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL:
            sn->sensor_reading.bool_value = *((uint8_t *)sensor_reading);
            break;

        case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32:
            sn->sensor_reading.uint32_value = *((uint32_t *)sensor_reading);
            break;

        case IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE:
            sn->sensor_reading.double_value = *((double *)sensor_reading);
            break;

        default:
        case IPMI_MONITORING_SENSOR_READING_TYPE_UNKNOWN:
            sn->do_metric = false;
            break;
    }
}

static void netdata_update_ipmi_sensor_reading(
        int record_id
        , int sensor_number
        , int sensor_type
        , int sensor_state
        , int sensor_units
        , int sensor_reading_type
        , char *sensor_name
        , void *sensor_reading
        , int event_reading_type_code __maybe_unused
        , int sensor_bitmask_type __maybe_unused
        , int sensor_bitmask __maybe_unused
        , char **sensor_bitmask_strings __maybe_unused
        , struct netdata_ipmi_state *stt) {
    if(unlikely(sensor_state == IPMI_MONITORING_STATE_UNKNOWN &&
        sensor_type == IPMI_MONITORING_SENSOR_TYPE_UNKNOWN &&
        sensor_units == IPMI_MONITORING_SENSOR_UNITS_UNKNOWN &&
        sensor_reading_type == IPMI_MONITORING_SENSOR_READING_TYPE_UNKNOWN &&
        (!sensor_name || !*sensor_name)))
        // we can't do anything about this sensor - everything is unknown
        return;

    if(unlikely(!sensor_name || !*sensor_name))
        sensor_name = "UNNAMED";

    stt->sensors.collected++;

    char key[SENSORS_DICT_KEY_SIZE + 1];
    snprintfz(key, SENSORS_DICT_KEY_SIZE, "i%d_n%d_t%d_u%d_%s",
              record_id, sensor_number, sensor_reading_type, sensor_units, sensor_name);

    // find the sensor record
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(stt->sensors.dict, key);
    if(likely(item)) {
        // recurring collection

        if(stt->debug)
            fprintf(stderr, "%s: reusing sensor record for sensor '%s', id %d, number %d, type %d, state %d, units %d, reading_type %d\n",
                    program_name, sensor_name, record_id, sensor_number, sensor_type, sensor_state, sensor_units, sensor_reading_type);

        struct sensor *sn = dictionary_acquired_item_value(item);

        if(sensor_reading) {
            netdata_sensor_set_value(sn, sensor_reading, stt);
            sn->last_collected_metric_ut = stt->sensors.now_ut;
        }

        sn->sensor_state = sensor_state;

        sn->last_collected_state_ut = stt->sensors.now_ut;

        dictionary_acquired_item_release(stt->sensors.dict, item);

        return;
    }

    if(stt->debug)
        fprintf(stderr, "Allocating new sensor data record for sensor '%s', id %d, number %d, type %d, state %d, units %d, reading_type %d\n",
                sensor_name, record_id, sensor_number, sensor_type, sensor_state, sensor_units, sensor_reading_type);

    // check if it is excluded
    bool excluded_metric = excluded_record_ids_check(record_id);
    bool excluded_state = excluded_status_record_ids_check(record_id);

    if(excluded_metric) {
        if(stt->debug)
            fprintf(stderr, "Sensor '%s' is excluded by excluded_record_ids_check()\n", sensor_name);
    }

    if(excluded_state) {
        if(stt->debug)
            fprintf(stderr, "Sensor '%s' is excluded for status check, by excluded_status_record_ids_check()\n", sensor_name);
    }

    struct sensor t = {
            .sensor_type = sensor_type,
            .sensor_state = sensor_state,
            .sensor_units = sensor_units,
            .sensor_reading_type = sensor_reading_type,
            .sensor_name = strdupz(sensor_name),
            .component = netdata_sensor_name_to_component(sensor_name),
            .do_state = !excluded_state,
            .do_metric = !excluded_metric,
    };

    t.type = netdata_ipmi_get_sensor_type_string(t.sensor_type, &t.component);

    switch(t.sensor_units) {
        case IPMI_MONITORING_SENSOR_UNITS_CELSIUS:
            t.dimension = "temperature";
            t.context = "ipmi.sensor_temperature_c";
            t.title = "IPMI Sensor Temperature Celsius";
            t.units = "Celsius";
            t.family = "temperatures";
            t.chart_type = "line";
            t.priority = stt->sensors.priority + 10;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_FAHRENHEIT:
            t.dimension = "temperature";
            t.context = "ipmi.sensor_temperature_f";
            t.title = "IPMI Sensor Temperature Fahrenheit";
            t.units = "Fahrenheit";
            t.family = "temperatures";
            t.chart_type = "line";
            t.priority = stt->sensors.priority + 20;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_VOLTS:
            t.dimension = "voltage";
            t.context = "ipmi.sensor_voltage";
            t.title = "IPMI Sensor Voltage";
            t.units = "Volts";
            t.family = "voltages";
            t.chart_type = "line";
            t.priority = stt->sensors.priority + 30;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_AMPS:
            t.dimension = "ampere";
            t.context = "ipmi.sensor_ampere";
            t.title = "IPMI Sensor Current";
            t.units = "Amps";
            t.family = "current";
            t.chart_type = "line";
            t.priority = stt->sensors.priority + 40;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_RPM:
            t.dimension = "rotations";
            t.context = "ipmi.sensor_fan_speed";
            t.title = "IPMI Sensor Fans Speed";
            t.units = "RPM";
            t.family = "fans";
            t.chart_type = "line";
            t.priority = stt->sensors.priority + 50;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_WATTS:
            t.dimension = "power";
            t.context = "ipmi.sensor_power";
            t.title = "IPMI Sensor Power";
            t.units = "Watts";
            t.family = "power";
            t.chart_type = "line";
            t.priority = stt->sensors.priority + 60;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_PERCENT:
            t.dimension = "percentage";
            t.context = "ipmi.sensor_reading_percent";
            t.title = "IPMI Sensor Reading Percentage";
            t.units = "%%";
            t.family = "other";
            t.chart_type = "line";
            t.priority = stt->sensors.priority + 70;
            break;

        default:
            t.priority = stt->sensors.priority + 80;
            t.do_metric = false;
            break;
    }

    switch(sensor_reading_type) {
        case IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE:
            t.multiplier = 1000;
            break;

        case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL:
        case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32:
            t.multiplier = 1;
            break;

        default:
            t.do_metric = false;
            break;
    }

    if(sensor_reading) {
        netdata_sensor_set_value(&t, sensor_reading, stt);
        t.last_collected_metric_ut = stt->sensors.now_ut;
    }
    t.last_collected_state_ut = stt->sensors.now_ut;

    dictionary_set(stt->sensors.dict, key, &t, sizeof(t));
}

static void netdata_update_ipmi_sel_events_count(struct netdata_ipmi_state *stt, uint32_t events) {
    stt->sel.events = events;
}

int netdata_ipmi_collect_data(struct ipmi_monitoring_ipmi_config *ipmi_config, IPMI_COLLECTION_TYPE type, struct netdata_ipmi_state *stt) {
    errno_clear();

    if(type & IPMI_COLLECT_TYPE_SENSORS) {
        stt->sensors.collected = 0;
        stt->sensors.now_ut = now_monotonic_usec();

        if (netdata_read_ipmi_sensors(ipmi_config, stt) < 0) return -1;
    }

    if(type & IPMI_COLLECT_TYPE_SEL) {
        stt->sel.events = 0;
        stt->sel.now_ut = now_monotonic_usec();
        if(netdata_get_ipmi_sel_events_count(ipmi_config, stt) < 0) return -2;
    }

    return 0;
}

int netdata_ipmi_detect_speed_secs(struct ipmi_monitoring_ipmi_config *ipmi_config, IPMI_COLLECTION_TYPE type, struct netdata_ipmi_state *stt) {
    int i, checks = SPEED_TEST_ITERATIONS, successful = 0;
    usec_t total = 0;

    for(i = 0 ; i < checks ; i++) {
        if(unlikely(stt->debug))
            fprintf(stderr, "%s: checking %s data collection speed iteration %d of %d\n",
                    program_name, netdata_collect_type_to_string(type), i + 1, checks);

        // measure the time a data collection needs
        usec_t start = now_realtime_usec();

        if(netdata_ipmi_collect_data(ipmi_config, type, stt) < 0)
            continue;

        usec_t end = now_realtime_usec();

        successful++;

        if(unlikely(stt->debug))
            fprintf(stderr, "%s: %s data collection speed was %"PRIu64" usec\n",
                    program_name, netdata_collect_type_to_string(type), end - start);

        // add it to our total
        total += end - start;

        // wait the same time
        // to avoid flooding the IPMI processor with requests
        sleep_usec(end - start);
    }

    if(!successful)
        return 0;

    // so, we assume it needed 2x the time
    // we find the average in microseconds
    // and we round-up to the closest second

    return (int)(( total * 2 / successful / USEC_PER_SEC ) + 1);
}

// ----------------------------------------------------------------------------
// data collection threads

struct ipmi_collection_thread {
    struct ipmi_monitoring_ipmi_config ipmi_config;
    int freq_s;
    bool debug;
    IPMI_COLLECTION_TYPE type;
    SPINLOCK spinlock;
    struct netdata_ipmi_state state;
};

void *netdata_ipmi_collection_thread(void *ptr) {
    struct ipmi_collection_thread *t = ptr;

    if(t->debug) fprintf(stderr, "%s: calling initialize_ipmi_config() for %s\n",
                         program_name, netdata_collect_type_to_string(t->type));

    initialize_ipmi_config(&t->ipmi_config);

    if(t->debug) fprintf(stderr, "%s: detecting IPMI minimum update frequency for %s...\n",
                         program_name, netdata_collect_type_to_string(t->type));

    int freq_s = netdata_ipmi_detect_speed_secs(&t->ipmi_config, t->type, &t->state);
    if(!freq_s) {
        if(t->type & IPMI_COLLECT_TYPE_SENSORS) {
            t->state.sensors.status = ICS_INIT_FAILED;
            t->state.sensors.last_iteration_ut = 0;
        }

        if(t->type & IPMI_COLLECT_TYPE_SEL) {
            t->state.sel.status = ICS_INIT_FAILED;
            t->state.sel.last_iteration_ut = 0;
        }

        return ptr;
    }
    else {
        if(t->type & IPMI_COLLECT_TYPE_SENSORS) {
            t->state.sensors.status = ICS_RUNNING;
        }

        if(t->type & IPMI_COLLECT_TYPE_SEL) {
            t->state.sel.status = ICS_RUNNING;
        }
    }

    t->freq_s = freq_s = MAX(t->freq_s, freq_s);

    if(t->debug) {
        fprintf(stderr, "%s: IPMI minimum update frequency of %s was calculated to %d seconds.\n",
                program_name, netdata_collect_type_to_string(t->type), t->freq_s);

        fprintf(stderr, "%s: starting data collection of %s\n",
                program_name, netdata_collect_type_to_string(t->type));
    }

    size_t iteration = 0, failures = 0;
    usec_t step = t->freq_s * USEC_PER_SEC;

    heartbeat_t hb;
    heartbeat_init(&hb, step);
    while(++iteration) {
        heartbeat_next(&hb);

        if(t->debug)
            fprintf(stderr, "%s: calling netdata_ipmi_collect_data() for %s\n",
                    program_name, netdata_collect_type_to_string(t->type));

        struct netdata_ipmi_state tmp_state = t->state;

        if(t->type & IPMI_COLLECT_TYPE_SENSORS) {
            tmp_state.sensors.last_iteration_ut = now_monotonic_usec();
            tmp_state.sensors.freq_ut = t->freq_s * USEC_PER_SEC;
        }

        if(t->type & IPMI_COLLECT_TYPE_SEL) {
            tmp_state.sel.last_iteration_ut = now_monotonic_usec();
            tmp_state.sel.freq_ut = t->freq_s * USEC_PER_SEC;
        }

        if(netdata_ipmi_collect_data(&t->ipmi_config, t->type, &tmp_state) != 0)
            failures++;
        else
            failures = 0;

        if(failures > 10) {
            collector_error("%s() failed to collect %s data for %zu consecutive times, having made %zu iterations.",
                            __FUNCTION__, netdata_collect_type_to_string(t->type), failures, iteration);

            if(t->type & IPMI_COLLECT_TYPE_SENSORS) {
                t->state.sensors.status = ICS_FAILED;
                t->state.sensors.last_iteration_ut = 0;
            }

            if(t->type & IPMI_COLLECT_TYPE_SEL) {
                t->state.sel.status = ICS_FAILED;
                t->state.sel.last_iteration_ut = 0;
            }

            break;
        }

        spinlock_lock(&t->spinlock);
        t->state = tmp_state;
        spinlock_unlock(&t->spinlock);
    }

    return ptr;
}

// ----------------------------------------------------------------------------
// sending data to netdata

static inline bool is_sensor_updated(usec_t last_collected_ut, usec_t now_ut, usec_t freq) {
    return (now_ut - last_collected_ut < freq * 2) ? true : false;
}

static size_t send_ipmi_sensor_metrics_to_netdata(struct netdata_ipmi_state *stt) {
    if(stt->sensors.status != ICS_RUNNING) {
        if(unlikely(stt->debug))
            fprintf(stderr, "%s: %s() sensors state is not RUNNING\n",
                    program_name, __FUNCTION__ );
        return 0;
    }

    size_t total_sensors_sent = 0;
    int update_every_s = (int)(stt->sensors.freq_ut / USEC_PER_SEC);
    struct sensor *sn;

    netdata_mutex_lock(&stdout_mutex);
    // generate the CHART/DIMENSION lines, if we have to
    dfe_start_reentrant(stt->sensors.dict, sn) {
                if(unlikely(!sn->do_metric && !sn->do_state))
                    continue;

                bool did_metric = false, did_state = false;

                if(likely(sn->do_metric)) {
                    if(unlikely(!is_sensor_updated(sn->last_collected_metric_ut, stt->updates.now_ut, stt->sensors.freq_ut))) {
                        if(unlikely(stt->debug))
                            fprintf(stderr, "%s: %s() sensor '%s' metric is not UPDATED (last updated %"PRIu64", now %"PRIu64", freq %"PRIu64"\n",
                                    program_name, __FUNCTION__, sn->sensor_name, sn->last_collected_metric_ut,
                                    stt->updates.now_ut, stt->sensors.freq_ut);
                    }
                    else {
                        if (unlikely(!sn->metric_chart_sent)) {
                            sn->metric_chart_sent = true;

                            printf("CHART '%s_%s' '' '%s' '%s' '%s' '%s' '%s' %d %d '' '%s' '%s'\n",
                                   sn->context, sn_dfe.name, sn->title, sn->units, sn->family, sn->context,
                                   sn->chart_type, sn->priority + 1,
                                   update_every_s, program_name, "sensors");

                            printf("CLABEL 'sensor' '%s' 1\n", sn->sensor_name);
                            printf("CLABEL 'type' '%s' 1\n", sn->type);
                            printf("CLABEL 'component' '%s' 1\n", sn->component);
                            printf("CLABEL_COMMIT\n");

                            printf("DIMENSION '%s' '' absolute 1 %d\n", sn->dimension, sn->multiplier);
                        }

                        printf("BEGIN '%s_%s'\n", sn->context, sn_dfe.name);

                        switch (sn->sensor_reading_type) {
                            case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32:
                                printf("SET '%s' = %u\n", sn->dimension, sn->sensor_reading.uint32_value);
                                break;

                            case IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE:
                                printf("SET '%s' = %lld\n", sn->dimension,
                                       (long long int) (sn->sensor_reading.double_value * sn->multiplier));
                                break;

                            case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL:
                                printf("SET '%s' = %u\n", sn->dimension, sn->sensor_reading.bool_value);
                                break;

                            default:
                            case IPMI_MONITORING_SENSOR_READING_TYPE_UNKNOWN:
                                // this should never happen because we also do the same check at netdata_get_sensor()
                                sn->do_metric = false;
                                break;
                        }

                        printf("END\n");
                        did_metric = true;
                    }
                }

                if(likely(sn->do_state)) {
                    if(unlikely(!is_sensor_updated(sn->last_collected_state_ut, stt->updates.now_ut, stt->sensors.freq_ut))) {
                        if (unlikely(stt->debug))
                            fprintf(stderr, "%s: %s() sensor '%s' state is not UPDATED (last updated %"PRIu64", now %"PRIu64", freq %"PRIu64"\n",
                                    program_name, __FUNCTION__, sn->sensor_name, sn->last_collected_state_ut,
                                    stt->updates.now_ut, stt->sensors.freq_ut);
                    }
                    else {
                        if (unlikely(!sn->state_chart_sent)) {
                            sn->state_chart_sent = true;

                            printf("CHART 'ipmi.sensor_state_%s' '' 'IPMI Sensor State' 'state' 'states' 'ipmi.sensor_state' 'line' %d %d '' '%s' '%s'\n",
                                   sn_dfe.name, sn->priority, update_every_s, program_name, "sensors");

                            printf("CLABEL 'sensor' '%s' 1\n", sn->sensor_name);
                            printf("CLABEL 'type' '%s' 1\n", sn->type);
                            printf("CLABEL 'component' '%s' 1\n", sn->component);
                            printf("CLABEL_COMMIT\n");

                            printf("DIMENSION 'nominal' '' absolute 1 1\n");
                            printf("DIMENSION 'warning' '' absolute 1 1\n");
                            printf("DIMENSION 'critical' '' absolute 1 1\n");
                            printf("DIMENSION 'unknown' '' absolute 1 1\n");
                        }

                        printf("BEGIN 'ipmi.sensor_state_%s'\n", sn_dfe.name);
                        printf("SET 'nominal' = %lld\n", sn->sensor_state == IPMI_MONITORING_STATE_NOMINAL ? 1LL : 0LL);
                        printf("SET 'warning' = %lld\n", sn->sensor_state == IPMI_MONITORING_STATE_WARNING ? 1LL : 0LL);
                        printf("SET 'critical' = %lld\n", sn->sensor_state == IPMI_MONITORING_STATE_CRITICAL ? 1LL : 0LL);
                        printf("SET 'unknown' = %lld\n", sn->sensor_state == IPMI_MONITORING_STATE_UNKNOWN ? 1LL : 0LL);
                        printf("END\n");
                        did_state = true;
                    }
                }

                if(likely(did_metric || did_state))
                    total_sensors_sent++;
            }
    dfe_done(sn);

    netdata_mutex_unlock(&stdout_mutex);

    return total_sensors_sent;
}

static size_t send_ipmi_sel_metrics_to_netdata(struct netdata_ipmi_state *stt) {
    static bool sel_chart_generated = false;

    netdata_mutex_lock(&stdout_mutex);

    if(likely(stt->sel.status == ICS_RUNNING)) {
        if(unlikely(!sel_chart_generated)) {
            sel_chart_generated = true;
            printf("CHART ipmi.events '' 'IPMI Events' 'events' 'events' ipmi.sel area %d %d '' '%s' '%s'\n"
                    , stt->sel.priority + 2
                    , (int)(stt->sel.freq_ut / USEC_PER_SEC)
                    , program_name
                    , "sel"
            );
            printf("DIMENSION events '' absolute 1 1\n");
        }

        printf(
                "BEGIN ipmi.events\n"
                "SET events = %zu\n"
                "END\n"
                ,
            stt->sel.events
        );
    }

    netdata_mutex_unlock(&stdout_mutex);

    return stt->sel.events;
}

// ----------------------------------------------------------------------------

static const char *get_sensor_state_string(struct sensor *sn) {
    switch (sn->sensor_state) {
        case IPMI_MONITORING_STATE_NOMINAL:
            return "nominal";
        case IPMI_MONITORING_STATE_WARNING:
            return "warning";
        case IPMI_MONITORING_STATE_CRITICAL:
            return "critical";
        default:
            return "unknown";
    }
}

static const char *get_sensor_function_priority(struct sensor *sn) {
    switch (sn->sensor_state) {
        case IPMI_MONITORING_STATE_WARNING:
            return "warning";
        case IPMI_MONITORING_STATE_CRITICAL:
            return "critical";
        default:
            return "normal";
    }
}

static void freeimi_function_sensors(const char *transaction, char *function __maybe_unused,
                                     usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
                                     BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
                                     const char *source __maybe_unused, void *data __maybe_unused) {
    time_t now_s = now_realtime_sec();

    BUFFER *wb = buffer_create(4096, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", update_every);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", "View IPMI sensor readings and its state");

    char function_copy[strlen(function) + 1];
    memcpy(function_copy, function, sizeof(function_copy));
    char *words[1024];
    size_t num_words = quoted_strings_splitter_whitespace(function_copy, words, 1024);
    for(size_t i = 1; i < num_words ;i++) {
        char *param = get_word(words, num_words, i);
        if(strcmp(param, "info") == 0) {
            buffer_json_member_add_array(wb, "accepted_params");
            buffer_json_array_close(wb); // accepted_params
            buffer_json_member_add_array(wb, "required_params");
            buffer_json_array_close(wb); // required_params
            goto close_and_send;
        }
    }

    buffer_json_member_add_array(wb, "data");

    struct sensor *sn;
    dfe_start_reentrant(state.sensors.dict, sn) {
        if (unlikely(!sn->do_metric && !sn->do_state))
            continue;

        double reading = NAN;
        switch (sn->sensor_reading_type) {
            case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32:
                        reading = (double)sn->sensor_reading.uint32_value;
                        break;
            case IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE:
                        reading = (double)(sn->sensor_reading.double_value);
                        break;
            case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL:
                        reading = (double)sn->sensor_reading.bool_value;
                        break;
        }

        buffer_json_add_array_item_array(wb);

        buffer_json_add_array_item_string(wb, sn->sensor_name);
        buffer_json_add_array_item_string(wb, sn->type);
        buffer_json_add_array_item_string(wb, sn->component);
        buffer_json_add_array_item_double(wb, reading);
        buffer_json_add_array_item_string(wb, sn->units);
        buffer_json_add_array_item_string(wb, get_sensor_state_string(sn));

        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "severity", get_sensor_function_priority(sn));
        buffer_json_object_close(wb);

        buffer_json_array_close(wb);
    }
    dfe_done(sn);

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "Sensor", "Sensor Name",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Type", "Sensor Type",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Component", "Sensor Component",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Reading", "Sensor Current Reading",
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                2, NULL, 0, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "Units", "Sensor Reading Units",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);
        buffer_rrdf_table_add_field(wb, field_id++, "State", "Sensor State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);
        buffer_rrdf_table_add_field(
                wb, field_id++,
                "rowOptions", "rowOptions",
                RRDF_FIELD_TYPE_NONE,
                RRDR_FIELD_VISUAL_ROW_OPTIONS,
                RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                RRDF_FIELD_SORT_FIXED,
                NULL,
                RRDF_FIELD_SUMMARY_COUNT,
                RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_DUMMY,
                NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Type");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Sensors");
        {
            buffer_json_member_add_string(wb, "name", "Sensors");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Sensor");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Sensors");
        buffer_json_add_array_item_string(wb, "Component");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Sensors");
        buffer_json_add_array_item_string(wb, "State");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

close_and_send:
    buffer_json_member_add_time_t(wb, "expires", now_s + update_every);
    buffer_json_finalize(wb);

    wb->response_code = HTTP_RESP_OK;
    wb->content_type = CT_APPLICATION_JSON;
    wb->expires = now_s + update_every;
    pluginsd_function_result_to_stdout(transaction, wb);

    buffer_free(wb);
}

// ----------------------------------------------------------------------------
// main, command line arguments parsing

static void plugin_exit(int code) {
    fflush(stdout);
    function_plugin_should_exit = true;
    exit(code);
}

int main (int argc, char **argv) {
    nd_log_initialize_for_external_plugins("freeipmi.plugin");
    netdata_threads_init_for_external_plugins(0); // set the default threads stack size here

    bool netdata_do_sel = IPMI_ENABLE_SEL_BY_DEFAULT;

    bool debug = false;

    // TODO: Workaround for https://github.com/netdata/netdata/issues/17931
    // This variable will be removed once the issue is fixed.
    bool restart_every = true;

    // ------------------------------------------------------------------------
    // parse command line parameters

    int i, freq_s = 0;
    for(i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !freq_s) {
            int n = str2i(argv[i]);
            if(n > 0 && n < 86400) {
                freq_s = n;
                continue;
            }
        }
        else if(strcmp("version", argv[i]) == 0 || strcmp("-version", argv[i]) == 0 || strcmp("--version", argv[i]) == 0 || strcmp("-v", argv[i]) == 0 || strcmp("-V", argv[i]) == 0) {
            printf("%s %s\n", program_name, NETDATA_VERSION);
            exit(0);
        }
        else if(strcmp("debug", argv[i]) == 0) {
            debug = true;
            continue;
        }
        else if(strcmp("no-restart", argv[i]) == 0) {
            restart_every = false;
            continue;
        }
        else if(strcmp("sel", argv[i]) == 0) {
            netdata_do_sel = true;
            continue;
        }
        else if(strcmp("no-sel", argv[i]) == 0) {
            netdata_do_sel = false;
            continue;
        }
        else if(strcmp("reread-sdr-cache", argv[i]) == 0) {
            global_sel_flags |= IPMI_MONITORING_SEL_FLAGS_REREAD_SDR_CACHE;
            global_sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_REREAD_SDR_CACHE;
            remove_reread_sdr_after_first_use = false;
            if (debug) fprintf(stderr, "%s: reread-sdr-cache enabled for both sensors and SEL\n", program_name);
        }
        else if(strcmp("interpret-oem-data", argv[i]) == 0) {
            global_sel_flags |= IPMI_MONITORING_SEL_FLAGS_INTERPRET_OEM_DATA;
            global_sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_INTERPRET_OEM_DATA;
            if (debug) fprintf(stderr, "%s: interpret-oem-data enabled for both sensors and SEL\n", program_name);
        }
        else if(strcmp("assume-system-event-record", argv[i]) == 0) {
            global_sel_flags |= IPMI_MONITORING_SEL_FLAGS_ASSUME_SYSTEM_EVENT_RECORD;
            if (debug) fprintf(stderr, "%s: assume-system-event-record enabled\n", program_name);
        }
        else if(strcmp("ignore-non-interpretable-sensors", argv[i]) == 0) {
            global_sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_IGNORE_NON_INTERPRETABLE_SENSORS;
            if (debug) fprintf(stderr, "%s: ignore-non-interpretable-sensors enabled\n", program_name);
        }
        else if(strcmp("bridge-sensors", argv[i]) == 0) {
            global_sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_BRIDGE_SENSORS;
            if (debug) fprintf(stderr, "%s: bridge-sensors enabled\n", program_name);
        }
        else if(strcmp("shared-sensors", argv[i]) == 0) {
            global_sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_SHARED_SENSORS;
            if (debug) fprintf(stderr, "%s: shared-sensors enabled\n", program_name);
        }
        else if(strcmp("no-discrete-reading", argv[i]) == 0) {
            global_sensor_reading_flags &= ~(IPMI_MONITORING_SENSOR_READING_FLAGS_DISCRETE_READING);
            if (debug) fprintf(stderr, "%s: discrete-reading disabled\n", program_name);
        }
        else if(strcmp("ignore-scanning-disabled", argv[i]) == 0) {
            global_sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_IGNORE_SCANNING_DISABLED;
            if (debug) fprintf(stderr, "%s: ignore-scanning-disabled enabled\n", program_name);
        }
        else if(strcmp("assume-bmc-owner", argv[i]) == 0) {
            global_sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_ASSUME_BMC_OWNER;
            if (debug) fprintf(stderr, "%s: assume-bmc-owner enabled\n", program_name);
        }
#if defined(IPMI_MONITORING_SEL_FLAGS_ENTITY_SENSOR_NAMES) && defined(IPMI_MONITORING_SENSOR_READING_FLAGS_ENTITY_SENSOR_NAMES)
        else if(strcmp("entity-sensor-names", argv[i]) == 0) {
            global_sel_flags |= IPMI_MONITORING_SEL_FLAGS_ENTITY_SENSOR_NAMES;
            global_sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_ENTITY_SENSOR_NAMES;
            if (debug) fprintf(stderr, "%s: entity-sensor-names enabled for both sensors and SEL\n", program_name);
        }
#endif
        else if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata %s %s\n"
                    " Copyright 2018-2025 Netdata Inc.\n"
                    " Released under GNU General Public License v3 or later.\n"
                    "\n"
                    " This program is a data collector plugin for netdata.\n"
                    "\n"
                    " Available command line options:\n"
                    "\n"
                    "  SECONDS                 data collection frequency\n"
                    "                          minimum: %d\n"
                    "\n"
                    "  debug                   enable verbose output\n"
                    "                          default: disabled\n"
                    "\n"
                    "  sel\n"
                    "  no-sel                  enable/disable SEL collection\n"
                    "                          default: %s\n"
                    "\n"
                    "  reread-sdr-cache        re-read SDR cache on every iteration\n"
                    "                          default: disabled\n"
                    "\n"
                    "  interpret-oem-data      attempt to parse OEM data\n"
                    "                          default: disabled\n"
                    "\n"
                    "  assume-system-event-record \n"
                    "                          tread illegal SEL events records as normal\n"
                    "                          default: disabled\n"
                    "\n"
                    "  ignore-non-interpretable-sensors \n"
                    "                          do not read sensors that cannot be interpreted\n"
                    "                          default: disabled\n"
                    "\n"
                    "  bridge-sensors          bridge sensors not owned by the BMC\n"
                    "                          default: disabled\n"
                    "\n"
                    "  shared-sensors          enable shared sensors, if found\n"
                    "                          default: disabled\n"
                    "\n"
                    "  no-discrete-reading     do not read sensors that their event/reading type code is invalid\n"
                    "                          default: enabled\n"
                    "\n"
                    "  ignore-scanning-disabled \n"
                    "                          Ignore the scanning bit and read sensors no matter what\n"
                    "                          default: disabled\n"
                    "\n"
                    "  assume-bmc-owner        assume the BMC is the sensor owner no matter what\n"
                    "                          (usually bridging is required too)\n"
                    "                          default: disabled\n"
                    "\n"
#if defined(IPMI_MONITORING_SEL_FLAGS_ENTITY_SENSOR_NAMES) && defined(IPMI_MONITORING_SENSOR_READING_FLAGS_ENTITY_SENSOR_NAMES)
                    "  entity-sensor-names     sensor names prefixed with entity id and instance\n"
                    "                          default: disabled\n"
                    "\n"
#endif
                    "  hostname HOST\n"
                    "  username USER\n"
                    "  password PASS           connect to remote IPMI host\n"
                    "                          default: local IPMI processor\n"
                    "\n"
                    "  no-auth-code-check\n"
                    "  noauthcodecheck         don't check the authentication codes returned\n"
                    "\n"
                    " driver-type IPMIDRIVER\n"
                    "                          Specify the driver type to use instead of doing an auto selection. \n"
                    "                          The currently available outofband drivers are LAN and LAN_2_0,\n"
                    "                          which  perform  IPMI  1.5  and  IPMI  2.0 respectively. \n"
                    "                          The currently available inband drivers are KCS, SSIF, OPENIPMI and SUNBMC.\n"
                    "\n"
                    "  sdr-cache-dir PATH      directory for SDR cache files\n"
                    "                          default: %s\n"
                    "\n"
                    "  sensor-config-file FILE filename to read sensor configuration\n"
                    "                          default: %s\n"
                    "\n"
                    "  sel-config-file FILE    filename to read sel configuration\n"
                    "                          default: %s\n"
                    "\n"
                    "  ignore N1,N2,N3,...     sensor IDs to ignore\n"
                    "                          default: none\n"
                    "\n"
                    "  ignore-status N1,N2,N3,... sensor IDs to ignore status (nominal/warning/critical)\n"
                    "                          default: none\n"
                    "\n"
                    "  -v\n"
                    "  -V\n"
                    "  version                 print version and exit\n"
                    "\n"
                    " Linux kernel module for IPMI is CPU hungry.\n"
                    " On Linux run this to lower kipmiN CPU utilization:\n"
                    " # echo 10 > /sys/module/ipmi_si/parameters/kipmid_max_busy_us\n"
                    "\n"
                    " or create: /etc/modprobe.d/ipmi.conf with these contents:\n"
                    " options ipmi_si kipmid_max_busy_us=10\n"
                    "\n"
                    " For more information:\n"
                    " https://github.com/netdata/netdata/tree/master/src/collectors/freeipmi.plugin\n"
                    "\n"
                    , program_name, NETDATA_VERSION
                    , update_every
                    , netdata_do_sel?"enabled":"disabled"
                    , sdr_cache_directory?sdr_cache_directory:"system default"
                    , sensor_config_file?sensor_config_file:"system default"
                    , sel_config_file?sel_config_file:"system default"
            );
            exit(1);
        }
        else if(i < argc && strcmp("hostname", argv[i]) == 0) {
            hostname = strdupz(argv[++i]);
            char *s = argv[i];
            // mask it be hidden from the process tree
            while(*s) *s++ = 'x';
            if(debug) fprintf(stderr, "%s: hostname set to '%s'\n", program_name, hostname);
            continue;
        }
        else if(i < argc && strcmp("username", argv[i]) == 0) {
            username = strdupz(argv[++i]);
            char *s = argv[i];
            // mask it be hidden from the process tree
            while(*s) *s++ = 'x';
            if(debug) fprintf(stderr, "%s: username set to '%s'\n", program_name, username);
            continue;
        }
        else if(i < argc && strcmp("password", argv[i]) == 0) {
            password = strdupz(argv[++i]);
            char *s = argv[i];
            // mask it be hidden from the process tree
            while(*s) *s++ = 'x';
            if(debug) fprintf(stderr, "%s: password set to '%s'\n", program_name, password);
            continue;
        }
        else if(strcmp("driver-type", argv[i]) == 0) {
            if (hostname) {
                freeimpi_protocol_version = netdata_parse_outofband_driver_type(argv[++i]);
                if(debug) fprintf(stderr, "%s: outband FreeIMPI protocol version set to '%d'\n",
                                  program_name, freeimpi_protocol_version);
            }
            else {
                driver_type = netdata_parse_inband_driver_type(argv[++i]);
                if(debug) fprintf(stderr, "%s: inband driver type set to '%d'\n",
                                  program_name, driver_type);
            }
            continue;
        } else if (i < argc && (strcmp("noauthcodecheck", argv[i]) == 0 || strcmp("no-auth-code-check", argv[i]) == 0)) {
            if (!hostname || netdata_host_is_localhost(hostname)) {
                if (debug)
                    fprintf(stderr, "%s: noauthcodecheck workaround flag is ignored for inband configuration\n",
                            program_name);

            }
            else if (freeimpi_protocol_version < 0 || freeimpi_protocol_version == IPMI_MONITORING_PROTOCOL_VERSION_1_5) {
                workaround_flags |= IPMI_MONITORING_WORKAROUND_FLAGS_PROTOCOL_VERSION_1_5_NO_AUTH_CODE_CHECK;

                if (debug)
                    fprintf(stderr, "%s: noauthcodecheck workaround flag enabled\n", program_name);
            }
            else {
                if (debug)
                    fprintf(stderr, "%s: noauthcodecheck workaround flag is ignored for protocol version 2.0\n",
                            program_name);
            }
            continue;
        }
        else if(i < argc && strcmp("sdr-cache-dir", argv[i]) == 0) {
            sdr_cache_directory = argv[++i];

            if(debug)
                fprintf(stderr, "%s: SDR cache directory set to '%s'\n", program_name, sdr_cache_directory);

            continue;
        }
        else if(i < argc && strcmp("sensor-config-file", argv[i]) == 0) {
            sensor_config_file = argv[++i];
            if(debug) fprintf(stderr, "%s: sensor config file set to '%s'\n", program_name, sensor_config_file);
            continue;
        }
        else if(i < argc && strcmp("sel-config-file", argv[i]) == 0) {
            sel_config_file = argv[++i];
            if(debug) fprintf(stderr, "%s: sel config file set to '%s'\n", program_name, sel_config_file);
            continue;
        }
        else if(i < argc && strcmp("ignore", argv[i]) == 0) {
            excluded_record_ids_parse(argv[++i], debug);
            continue;
        }
        else if(i < argc && strcmp("ignore-status", argv[i]) == 0) {
            excluded_status_record_ids_parse(argv[++i], debug);
            continue;
        }

        collector_error("%s(): ignoring parameter '%s'", __FUNCTION__, argv[i]);
    }

    errno_clear();

    if(freq_s && freq_s < update_every)
        collector_info("%s(): update frequency %d seconds is too small for IPMI. Using %d.",
                        __FUNCTION__, freq_s, update_every);

    update_every = freq_s = MAX(freq_s, update_every);
    update_every_sel = MAX(update_every, update_every_sel);

    // ------------------------------------------------------------------------
    // initialize IPMI

    if(debug) {
        fprintf(stderr, "%s: calling ipmi_monitoring_init()\n", program_name);
        ipmimonitoring_init_flags |= IPMI_MONITORING_FLAGS_DEBUG|IPMI_MONITORING_FLAGS_DEBUG_IPMI_PACKETS;
    }

    int rc;
    if(ipmi_monitoring_init(ipmimonitoring_init_flags, &rc) < 0)
        fatal("ipmi_monitoring_init: %s", ipmi_monitoring_ctx_strerror(rc));

    // ------------------------------------------------------------------------
    // create the data collection threads

    struct ipmi_collection_thread sensors_data = {
            .type = IPMI_COLLECT_TYPE_SENSORS,
            .freq_s = update_every,
            .spinlock = SPINLOCK_INITIALIZER,
            .debug = debug,
            .state = {
                    .debug = debug,
                    .sensors = {
                            .status = ICS_INIT,
                            .last_iteration_ut = now_monotonic_usec(),
                            .freq_ut = update_every * USEC_PER_SEC,
                            .priority = IPMI_SENSORS_DASHBOARD_PRIORITY,
                            .dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct sensor)),
                    },
            },
    }, sel_data = {
            .type = IPMI_COLLECT_TYPE_SEL,
            .freq_s = update_every_sel,
            .spinlock = SPINLOCK_INITIALIZER,
            .debug = debug,
            .state = {
                    .debug = debug,
                    .sel = {
                            .status = ICS_INIT,
                            .last_iteration_ut = now_monotonic_usec(),
                            .freq_ut = update_every_sel * USEC_PER_SEC,
                            .priority = IPMI_SEL_DASHBOARD_PRIORITY,
                    },
            },
    };

    nd_thread_create("IPMI[sensors]", NETDATA_THREAD_OPTION_DONT_LOG, netdata_ipmi_collection_thread, &sensors_data);
    if(netdata_do_sel)
        nd_thread_create("IPMI[sel]", NETDATA_THREAD_OPTION_DONT_LOG, netdata_ipmi_collection_thread, &sel_data);

    // ------------------------------------------------------------------------
    // the main loop

    if(debug) fprintf(stderr, "%s: starting data collection\n", program_name);

    time_t started_t = now_monotonic_sec();

    size_t iteration = 0;
    bool global_chart_created = false;
    bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    for(iteration = 0; 1 ; iteration++) {
        usec_t dt = heartbeat_next(&hb);

        if (!tty) {
            netdata_mutex_lock(&stdout_mutex);
            fprintf(stdout, "\n"); // keepalive to avoid parser read timeout (2 minutes) during ipmi_detect_speed_secs()
            fflush(stdout);
            netdata_mutex_unlock(&stdout_mutex);
        }

        spinlock_lock(&sensors_data.spinlock);
        state.sensors = sensors_data.state.sensors;
        spinlock_unlock(&sensors_data.spinlock);

        spinlock_lock(&sel_data.spinlock);
        state.sel = sel_data.state.sel;
        spinlock_unlock(&sel_data.spinlock);

        switch(state.sensors.status) {
            case ICS_RUNNING:
                if(state.sensors.last_iteration_ut < now_monotonic_usec() - IPMI_RESTART_IF_SENSORS_DONT_ITERATE_EVERY_SECONDS * USEC_PER_SEC) {
                    collector_error("%s(): sensors have not be collected for %zu seconds. Exiting to restart.",
                                    __FUNCTION__, (size_t)((now_monotonic_usec() - state.sensors.last_iteration_ut) / USEC_PER_SEC));

                    fprintf(stdout, "EXIT\n");
                    plugin_exit(0);
                }
                break;

            case ICS_INIT:
                continue;

            case ICS_INIT_FAILED:
                collector_error("%s(): sensors failed to initialize. Calling DISABLE.", __FUNCTION__);
                fprintf(stdout, "DISABLE\n");
                plugin_exit(0);
                break;

            case ICS_FAILED:
                collector_error("%s(): sensors fails repeatedly to collect metrics. Exiting to restart.", __FUNCTION__);
                fprintf(stdout, "EXIT\n");
                plugin_exit(0);
                break;
        }

        if(netdata_do_sel) {
            switch (state.sensors.status) {
                case ICS_RUNNING:
                case ICS_INIT:
                    break;

                case ICS_INIT_FAILED:
                case ICS_FAILED:
                    collector_error("%s(): SEL fails to collect events. Disabling SEL collection.", __FUNCTION__);
                    netdata_do_sel = false;
                    break;
            }
        }

        if(unlikely(debug))
            fprintf(stderr, "%s: calling send_ipmi_sensor_metrics_to_netdata()\n", program_name);

        static bool add_func_sensors = true;
        if (add_func_sensors) {
            add_func_sensors = false;
            struct functions_evloop_globals *wg =
                functions_evloop_init(1, "FREEIPMI", &stdout_mutex, &function_plugin_should_exit);
            functions_evloop_add_function(
                wg, "ipmi-sensors", freeimi_function_sensors, PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT, NULL);
            FREEIPMI_GLOBAL_FUNCTION_SENSORS();
        }

        state.updates.now_ut = now_monotonic_usec();
        send_ipmi_sensor_metrics_to_netdata(&state);

        if(netdata_do_sel)
            send_ipmi_sel_metrics_to_netdata(&state);

        if(unlikely(debug))
            fprintf(stderr, "%s: iteration %zu, dt %"PRIu64" usec, sensors ever collected %zu, sensors last collected %zu \n"
                    , program_name
                    , iteration
                    , dt
                    , dictionary_entries(state.sensors.dict)
                    , state.sensors.collected
            );

        netdata_mutex_lock(&stdout_mutex);

        if (!global_chart_created) {
            global_chart_created = true;

            fprintf(stdout,
                    "CHART netdata.freeipmi_availability_status '' 'Plugin availability status' 'status' "
                    "plugins netdata.plugin_availability_status line 146000 %d '' '%s' '%s'\n"
                    "DIMENSION available '' absolute 1 1\n",
                    update_every, program_name, "");
        }

        fprintf(stdout,
                "BEGIN netdata.freeipmi_availability_status\n"
                "SET available = 1\n"
                "END\n");

        // restart check (14400 seconds)
        if (restart_every && (now_monotonic_sec() - started_t > IPMI_RESTART_EVERY_SECONDS)) {
            collector_info("%s(): reached my lifetime expectancy. Exiting to restart.", __FUNCTION__);
            fprintf(stdout, "EXIT\n");
            plugin_exit(0);
        }

        fflush(stdout);

        netdata_mutex_unlock(&stdout_mutex);
    }
}
