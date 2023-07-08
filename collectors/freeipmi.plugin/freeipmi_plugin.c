// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata freeipmi.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 *
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

// forward definition of functions and structures
struct netdata_ipmi_state;
static void netdata_get_sensor(
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
        , struct netdata_ipmi_state *state
);
static void netdata_get_sel(int record_id, int record_type_class, int sel_state, struct netdata_ipmi_state *state);

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

#include <ipmi_monitoring.h>
#include <ipmi_monitoring_bitmasks.h>

/* Communication Configuration - Initialize accordingly */

/* Hostname, NULL for In-band communication, non-null for a hostname */
char *hostname = NULL;

/* In-band Communication Configuration */
int driver_type = -1; // IPMI_MONITORING_DRIVER_TYPE_KCS; /* or -1 for default */
int disable_auto_probe = 0;     /* probe for in-band device */
unsigned int driver_address = 0; /* not used if probing */
unsigned int register_spacing = 0; /* not used if probing */
char *driver_device = NULL;     /* not used if probing */

/* Out-of-band Communication Configuration */
int protocol_version = -1; //IPMI_MONITORING_PROTOCOL_VERSION_1_5; /* or -1 for default */
char *username = "foousername";
char *password = "foopassword";
unsigned char *k_g = NULL;
unsigned int k_g_len = 0;
int privilege_level = -1; // IPMI_MONITORING_PRIVILEGE_LEVEL_USER; /* or -1 for default */
int authentication_type = -1; // IPMI_MONITORING_AUTHENTICATION_TYPE_MD5; /* or -1 for default */
int cipher_suite_id = 0;        /* or -1 for default */
int session_timeout = 0;        /* 0 for default */
int retransmission_timeout = 0; /* 0 for default */

/* Workarounds - specify workaround flags if necessary */
unsigned int workaround_flags = 0;

/* Initialize w/ record id numbers to only monitor specific record ids */
unsigned int record_ids[] = {0};
unsigned int record_ids_length = 0;

/* Initialize w/ sensor types to only monitor specific sensor types
 * see ipmi_monitoring.h sensor types list.
 */
unsigned int sensor_types[] = {0};
unsigned int sensor_types_length = 0;

/* Set to an appropriate alternate if desired */
char *sdr_cache_directory = "/tmp";
char *sensor_config_file = NULL;

/* Set to 1 or 0 to enable these sensor reading flags
 * - See ipmi_monitoring.h for descriptions of these flags.
 */
int reread_sdr_cache = 0;
int ignore_non_interpretable_sensors = 0;
int bridge_sensors = 0;
int interpret_oem_data = 0;
int shared_sensors = 0;
int discrete_reading = 1;
int ignore_scanning_disabled = 0;
int assume_bmc_owner = 0;
int entity_sensor_names = 0;

/* Initialization flags
 *
 * Most commonly bitwise OR IPMI_MONITORING_FLAGS_DEBUG and/or
 * IPMI_MONITORING_FLAGS_DEBUG_IPMI_PACKETS for extra debugging
 * information.
 */
unsigned int ipmimonitoring_init_flags = 0;

int errnum;

// ----------------------------------------------------------------------------
// SEL only variables

/* Initialize w/ date range to only monitoring specific date range */
char *date_begin = NULL;        /* use MM/DD/YYYY format */
char *date_end = NULL;          /* use MM/DD/YYYY format */

int assume_system_event_record = 0;

char *sel_config_file = NULL;


// ----------------------------------------------------------------------------
// functions common to sensors and SEL

static void
_init_ipmi_config (struct ipmi_monitoring_ipmi_config *ipmi_config)
{
    fatal_assert(ipmi_config);

    ipmi_config->driver_type = driver_type;
    ipmi_config->disable_auto_probe = disable_auto_probe;
    ipmi_config->driver_address = driver_address;
    ipmi_config->register_spacing = register_spacing;
    ipmi_config->driver_device = driver_device;

    ipmi_config->protocol_version = protocol_version;
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

static const char *
_get_sensor_type_string (int sensor_type)
{
    switch (sensor_type)
    {
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
            return ("Physical Security");
        case IPMI_MONITORING_SENSOR_TYPE_PLATFORM_SECURITY_VIOLATION_ATTEMPT:
            return ("Platform Security Violation Attempt");
        case IPMI_MONITORING_SENSOR_TYPE_PROCESSOR:
            return ("Processor");
        case IPMI_MONITORING_SENSOR_TYPE_POWER_SUPPLY:
            return ("Power Supply");
        case IPMI_MONITORING_SENSOR_TYPE_POWER_UNIT:
            return ("Power Unit");
        case IPMI_MONITORING_SENSOR_TYPE_COOLING_DEVICE:
            return ("Cooling Device");
        case IPMI_MONITORING_SENSOR_TYPE_OTHER_UNITS_BASED_SENSOR:
            return ("Other Units Based Sensor");
        case IPMI_MONITORING_SENSOR_TYPE_MEMORY:
            return ("Memory");
        case IPMI_MONITORING_SENSOR_TYPE_DRIVE_SLOT:
            return ("Drive Slot");
        case IPMI_MONITORING_SENSOR_TYPE_POST_MEMORY_RESIZE:
            return ("POST Memory Resize");
        case IPMI_MONITORING_SENSOR_TYPE_SYSTEM_FIRMWARE_PROGRESS:
            return ("System Firmware Progress");
        case IPMI_MONITORING_SENSOR_TYPE_EVENT_LOGGING_DISABLED:
            return ("Event Logging Disabled");
        case IPMI_MONITORING_SENSOR_TYPE_WATCHDOG1:
            return ("Watchdog 1");
        case IPMI_MONITORING_SENSOR_TYPE_SYSTEM_EVENT:
            return ("System Event");
        case IPMI_MONITORING_SENSOR_TYPE_CRITICAL_INTERRUPT:
            return ("Critical Interrupt");
        case IPMI_MONITORING_SENSOR_TYPE_BUTTON_SWITCH:
            return ("Button/Switch");
        case IPMI_MONITORING_SENSOR_TYPE_MODULE_BOARD:
            return ("Module/Board");
        case IPMI_MONITORING_SENSOR_TYPE_MICROCONTROLLER_COPROCESSOR:
            return ("Microcontroller/Coprocessor");
        case IPMI_MONITORING_SENSOR_TYPE_ADD_IN_CARD:
            return ("Add In Card");
        case IPMI_MONITORING_SENSOR_TYPE_CHASSIS:
            return ("Chassis");
        case IPMI_MONITORING_SENSOR_TYPE_CHIP_SET:
            return ("Chip Set");
        case IPMI_MONITORING_SENSOR_TYPE_OTHER_FRU:
            return ("Other Fru");
        case IPMI_MONITORING_SENSOR_TYPE_CABLE_INTERCONNECT:
            return ("Cable/Interconnect");
        case IPMI_MONITORING_SENSOR_TYPE_TERMINATOR:
            return ("Terminator");
        case IPMI_MONITORING_SENSOR_TYPE_SYSTEM_BOOT_INITIATED:
            return ("System Boot Initiated");
        case IPMI_MONITORING_SENSOR_TYPE_BOOT_ERROR:
            return ("Boot Error");
        case IPMI_MONITORING_SENSOR_TYPE_OS_BOOT:
            return ("OS Boot");
        case IPMI_MONITORING_SENSOR_TYPE_OS_CRITICAL_STOP:
            return ("OS Critical Stop");
        case IPMI_MONITORING_SENSOR_TYPE_SLOT_CONNECTOR:
            return ("Slot/Connector");
        case IPMI_MONITORING_SENSOR_TYPE_SYSTEM_ACPI_POWER_STATE:
            return ("System ACPI Power State");
        case IPMI_MONITORING_SENSOR_TYPE_WATCHDOG2:
            return ("Watchdog 2");
        case IPMI_MONITORING_SENSOR_TYPE_PLATFORM_ALERT:
            return ("Platform Alert");
        case IPMI_MONITORING_SENSOR_TYPE_ENTITY_PRESENCE:
            return ("Entity Presence");
        case IPMI_MONITORING_SENSOR_TYPE_MONITOR_ASIC_IC:
            return ("Monitor ASIC/IC");
        case IPMI_MONITORING_SENSOR_TYPE_LAN:
            return ("LAN");
        case IPMI_MONITORING_SENSOR_TYPE_MANAGEMENT_SUBSYSTEM_HEALTH:
            return ("Management Subsystem Health");
        case IPMI_MONITORING_SENSOR_TYPE_BATTERY:
            return ("Battery");
        case IPMI_MONITORING_SENSOR_TYPE_SESSION_AUDIT:
            return ("Session Audit");
        case IPMI_MONITORING_SENSOR_TYPE_VERSION_CHANGE:
            return ("Version Change");
        case IPMI_MONITORING_SENSOR_TYPE_FRU_STATE:
            return ("FRU State");
    }

    return ("Unrecognized");
}

#define ipmi_sensor_read_int(var, func, ctx) do {               \
    if(( (var) = func(ctx) < 0 )) {                             \
        collector_error("%s(): call to " #func " failed: %s",   \
            __FUNCTION__, ipmi_monitoring_ctx_errormsg(ctx));   \
        goto cleanup;                                           \
    }                                                           \
    timing_step(TIMING_STEP_FREEIPMI_READ_ ## var);             \
} while(0)

#define ipmi_sensor_read_str(var, func, ctx) do {               \
    if(!( (var) = func(ctx) )) {                                \
        collector_error("%s(): call to " #func " failed: %s",   \
            __FUNCTION__, ipmi_monitoring_ctx_errormsg(ctx));   \
        goto cleanup;                                           \
    }                                                           \
    timing_step(TIMING_STEP_FREEIPMI_READ_ ## var);             \
} while(0)

#define ipmi_sensor_read_no_check(var, func, ctx) do {          \
    (var) = func(ctx);                                          \
    timing_step(TIMING_STEP_FREEIPMI_READ_ ## var);             \
} while(0)

static int
_ipmimonitoring_sensors (struct ipmi_monitoring_ipmi_config *ipmi_config, struct netdata_ipmi_state *state)
{
    timing_init();

    ipmi_monitoring_ctx_t ctx = NULL;
    unsigned int sensor_reading_flags = 0;
    int i;
    int sensor_count;
    int rv = -1;

    if (!(ctx = ipmi_monitoring_ctx_create ())) {
        collector_error("ipmi_monitoring_ctx_create()");
        goto cleanup;
    }

    timing_step(TIMING_STEP_FREEIPMI_CTX_CREATE);

    if (sdr_cache_directory)
    {
        if (ipmi_monitoring_ctx_sdr_cache_directory (ctx,
                sdr_cache_directory) < 0) {
            collector_error("ipmi_monitoring_ctx_sdr_cache_directory(): %s\n",
                            ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

    timing_step(TIMING_STEP_FREEIPMI_DSR_CACHE_DIR);

    // Must call otherwise only default interpretations ever used
    // sensor_config_file can be NULL
    if (ipmi_monitoring_ctx_sensor_config_file (ctx, sensor_config_file) < 0) {
        collector_error( "ipmi_monitoring_ctx_sensor_config_file(): %s\n",
                         ipmi_monitoring_ctx_errormsg (ctx));
        goto cleanup;
    }

    timing_step(TIMING_STEP_FREEIPMI_SENSOR_CONFIG_FILE);

    if (reread_sdr_cache)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_REREAD_SDR_CACHE;

    if (ignore_non_interpretable_sensors)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_IGNORE_NON_INTERPRETABLE_SENSORS;

    if (bridge_sensors)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_BRIDGE_SENSORS;

    if (interpret_oem_data)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_INTERPRET_OEM_DATA;

    if (shared_sensors)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_SHARED_SENSORS;

    if (discrete_reading)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_DISCRETE_READING;

    if (ignore_scanning_disabled)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_IGNORE_SCANNING_DISABLED;

    if (assume_bmc_owner)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_ASSUME_BMC_OWNER;

#ifdef IPMI_MONITORING_SENSOR_READING_FLAGS_ENTITY_SENSOR_NAMES
    if (entity_sensor_names)
        sensor_reading_flags |= IPMI_MONITORING_SENSOR_READING_FLAGS_ENTITY_SENSOR_NAMES;
#endif // IPMI_MONITORING_SENSOR_READING_FLAGS_ENTITY_SENSOR_NAMES

    if (!record_ids_length && !sensor_types_length) {
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
    }
    else if (record_ids_length) {
        if ((sensor_count = ipmi_monitoring_sensor_readings_by_record_id (ctx,
                hostname,
                ipmi_config,
                sensor_reading_flags,
                record_ids,
                record_ids_length,
                NULL,
                NULL)) < 0) {
            collector_error( "ipmi_monitoring_sensor_readings_by_record_id(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else {
        if ((sensor_count = ipmi_monitoring_sensor_readings_by_sensor_type (ctx,
                hostname,
                ipmi_config,
                sensor_reading_flags,
                sensor_types,
                sensor_types_length,
                NULL,
                NULL)) < 0) {
            collector_error( "ipmi_monitoring_sensor_readings_by_sensor_type(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

    timing_step(TIMING_STEP_FREEIPMI_SENSOR_READINGS_BY_X);

    for (i = 0; i < sensor_count; i++, ipmi_monitoring_sensor_iterator_next (ctx)) {
        int record_id, sensor_number, sensor_type, sensor_state, sensor_units,
            sensor_bitmask_type, sensor_bitmask, event_reading_type_code, sensor_reading_type;

        char **sensor_bitmask_strings = NULL;
        char *sensor_name = NULL;
        void *sensor_reading;

        ipmi_sensor_read_int(record_id, ipmi_monitoring_sensor_read_record_id, ctx);
        ipmi_sensor_read_int(sensor_number, ipmi_monitoring_sensor_read_sensor_number, ctx);
        ipmi_sensor_read_int(sensor_type, ipmi_monitoring_sensor_read_sensor_type, ctx);
        ipmi_sensor_read_str(sensor_name, ipmi_monitoring_sensor_read_sensor_name, ctx);
        ipmi_sensor_read_int(sensor_state, ipmi_monitoring_sensor_read_sensor_state, ctx);
        ipmi_sensor_read_int(sensor_units, ipmi_monitoring_sensor_read_sensor_units, ctx);
        ipmi_sensor_read_int(sensor_bitmask_type, ipmi_monitoring_sensor_read_sensor_bitmask_type, ctx);
        ipmi_sensor_read_int(sensor_bitmask, ipmi_monitoring_sensor_read_sensor_bitmask, ctx);
        // it's ok for this to be NULL, i.e. sensor_bitmask == IPMI_MONITORING_SENSOR_BITMASK_TYPE_UNKNOWN
        ipmi_sensor_read_no_check(sensor_bitmask_strings, ipmi_monitoring_sensor_read_sensor_bitmask_strings, ctx);
        ipmi_sensor_read_int(sensor_reading_type, ipmi_monitoring_sensor_read_sensor_reading_type, ctx);
        // whatever we read for the sensor, it is ok
        ipmi_sensor_read_no_check(sensor_reading, ipmi_monitoring_sensor_read_sensor_reading, ctx);
        ipmi_sensor_read_int(event_reading_type_code, ipmi_monitoring_sensor_read_event_reading_type_code, ctx);

        netdata_get_sensor(
                record_id
                , sensor_number
                , sensor_type
                , sensor_state
                , sensor_units
                , sensor_reading_type
                , sensor_name
                , sensor_reading
                , event_reading_type_code
                , sensor_bitmask_type
                , sensor_bitmask
                , sensor_bitmask_strings
                , state
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

    return (rv);
}


static int
_ipmimonitoring_sel (struct ipmi_monitoring_ipmi_config *ipmi_config, struct netdata_ipmi_state *state)
{
    ipmi_monitoring_ctx_t ctx = NULL;
    unsigned int sel_flags = 0;
    int i;
    int sel_count;
    int rv = -1;

    if (!(ctx = ipmi_monitoring_ctx_create ()))
    {
        collector_error("ipmi_monitoring_ctx_create()");
        goto cleanup;
    }

    if (sdr_cache_directory)
    {
        if (ipmi_monitoring_ctx_sdr_cache_directory (ctx,
                sdr_cache_directory) < 0)
        {
            collector_error( "ipmi_monitoring_ctx_sdr_cache_directory(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

    /* Must call otherwise only default interpretations ever used */
    if (sel_config_file)
    {
        if (ipmi_monitoring_ctx_sel_config_file (ctx,
                sel_config_file) < 0)
        {
            collector_error( "ipmi_monitoring_ctx_sel_config_file(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else
    {
        if (ipmi_monitoring_ctx_sel_config_file (ctx, NULL) < 0)
        {
            collector_error( "ipmi_monitoring_ctx_sel_config_file(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

    if (reread_sdr_cache)
        sel_flags |= IPMI_MONITORING_SEL_FLAGS_REREAD_SDR_CACHE;

    if (interpret_oem_data)
        sel_flags |= IPMI_MONITORING_SEL_FLAGS_INTERPRET_OEM_DATA;

    if (assume_system_event_record)
        sel_flags |= IPMI_MONITORING_SEL_FLAGS_ASSUME_SYSTEM_EVENT_RECORD;

#ifdef IPMI_MONITORING_SEL_FLAGS_ENTITY_SENSOR_NAMES
    if (entity_sensor_names)
        sel_flags |= IPMI_MONITORING_SEL_FLAGS_ENTITY_SENSOR_NAMES;
#endif // IPMI_MONITORING_SEL_FLAGS_ENTITY_SENSOR_NAMES

    if (record_ids_length)
    {
        if ((sel_count = ipmi_monitoring_sel_by_record_id (ctx,
                hostname,
                ipmi_config,
                sel_flags,
                record_ids,
                record_ids_length,
                NULL,
                NULL)) < 0)
        {
            collector_error( "ipmi_monitoring_sel_by_record_id(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else if (sensor_types_length)
    {
        if ((sel_count = ipmi_monitoring_sel_by_sensor_type (ctx,
                hostname,
                ipmi_config,
                sel_flags,
                sensor_types,
                sensor_types_length,
                NULL,
                NULL)) < 0)
        {
            collector_error( "ipmi_monitoring_sel_by_sensor_type(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else if (date_begin
             || date_end)
    {
        if ((sel_count = ipmi_monitoring_sel_by_date_range (ctx,
                hostname,
                ipmi_config,
                sel_flags,
                date_begin,
                date_end,
                NULL,
                NULL)) < 0)
        {
            collector_error( "ipmi_monitoring_sel_by_sensor_type(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else
    {
        if ((sel_count = ipmi_monitoring_sel_by_record_id (ctx,
                hostname,
                ipmi_config,
                sel_flags,
                NULL,
                0,
                NULL,
                NULL)) < 0)
        {
            collector_error( "ipmi_monitoring_sel_by_record_id(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

#ifdef NETDATA_COMMENTED
    printf ("%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s\n",
            "Record ID",
            "Record Type",
            "SEL State",
            "Timestamp",
            "Sensor Name",
            "Sensor Type",
            "Event Direction",
            "Event Type Code",
            "Event Data",
            "Event Offset",
            "Event Offset String");
#endif // NETDATA_COMMENTED

    for (i = 0; i < sel_count; i++, ipmi_monitoring_sel_iterator_next (ctx))
    {
        int record_id, record_type, sel_state, record_type_class;
#ifdef NETDATA_COMMENTED
        int sensor_type, sensor_number, event_direction,
                event_offset_type, event_offset, event_type_code, manufacturer_id;
        unsigned int timestamp, event_data1, event_data2, event_data3;
        char *event_offset_string = NULL;
        const char *sensor_type_str;
        const char *event_direction_str;
        const char *sel_state_str;
        char *sensor_name = NULL;
        unsigned char oem_data[64];
        int oem_data_len;
        unsigned int j;
#endif // NETDATA_COMMENTED

        if ((record_id = ipmi_monitoring_sel_read_record_id (ctx)) < 0)
        {
            collector_error( "ipmi_monitoring_sel_read_record_id(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((record_type = ipmi_monitoring_sel_read_record_type (ctx)) < 0)
        {
            collector_error( "ipmi_monitoring_sel_read_record_type(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((record_type_class = ipmi_monitoring_sel_read_record_type_class (ctx)) < 0)
        {
            collector_error( "ipmi_monitoring_sel_read_record_type_class(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sel_state = ipmi_monitoring_sel_read_sel_state (ctx)) < 0)
        {
            collector_error( "ipmi_monitoring_sel_read_sel_state(): %s",
                             ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        netdata_get_sel(
                record_id
                , record_type_class
                , sel_state
                , state
        );

#ifdef NETDATA_COMMENTED
        if (sel_state == IPMI_MONITORING_STATE_NOMINAL)
            sel_state_str = "Nominal";
        else if (sel_state == IPMI_MONITORING_STATE_WARNING)
            sel_state_str = "Warning";
        else if (sel_state == IPMI_MONITORING_STATE_CRITICAL)
            sel_state_str = "Critical";
        else
            sel_state_str = "N/A";

        printf ("%d, %d, %s",
                record_id,
                record_type,
                sel_state_str);

        if (record_type_class == IPMI_MONITORING_SEL_RECORD_TYPE_CLASS_SYSTEM_EVENT_RECORD
            || record_type_class == IPMI_MONITORING_SEL_RECORD_TYPE_CLASS_TIMESTAMPED_OEM_RECORD)
        {

            if (ipmi_monitoring_sel_read_timestamp (ctx, &timestamp) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_timestamp(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            /* XXX: This should be converted to a nice date output using
             * your favorite timestamp -> string conversion functions.
             */
            printf (", %u", timestamp);
        }
        else
            printf (", N/A");

        if (record_type_class == IPMI_MONITORING_SEL_RECORD_TYPE_CLASS_SYSTEM_EVENT_RECORD)
        {
            /* If you are integrating ipmimonitoring SEL into a monitoring application,
             * you may wish to count the number of times a specific error occurred
             * and report that to the monitoring application.
             *
             * In this particular case, you'll probably want to check out
             * what sensor type each SEL event is reporting, the
             * event offset type, and the specific event offset that occurred.
             *
             * See ipmi_monitoring_offsets.h for a list of event offsets
             * and types.
             */

            if (!(sensor_name = ipmi_monitoring_sel_read_sensor_name (ctx)))
            {
                collector_error( "ipmi_monitoring_sel_read_sensor_name(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((sensor_type = ipmi_monitoring_sel_read_sensor_type (ctx)) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_sensor_type(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((sensor_number = ipmi_monitoring_sel_read_sensor_number (ctx)) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_sensor_number(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_direction = ipmi_monitoring_sel_read_event_direction (ctx)) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_event_direction(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_type_code = ipmi_monitoring_sel_read_event_type_code (ctx)) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_event_type_code(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if (ipmi_monitoring_sel_read_event_data (ctx,
                    &event_data1,
                    &event_data2,
                    &event_data3) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_event_data(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_offset_type = ipmi_monitoring_sel_read_event_offset_type (ctx)) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_event_offset_type(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_offset = ipmi_monitoring_sel_read_event_offset (ctx)) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_event_offset(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if (!(event_offset_string = ipmi_monitoring_sel_read_event_offset_string (ctx)))
            {
                collector_error( "ipmi_monitoring_sel_read_event_offset_string(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if (!strlen (sensor_name))
                sensor_name = "N/A";

            sensor_type_str = _get_sensor_type_string (sensor_type);

            if (event_direction == IPMI_MONITORING_SEL_EVENT_DIRECTION_ASSERTION)
                event_direction_str = "Assertion";
            else
                event_direction_str = "Deassertion";

            printf (", %s, %s, %d, %s, %Xh, %Xh-%Xh-%Xh",
                    sensor_name,
                    sensor_type_str,
                    sensor_number,
                    event_direction_str,
                    event_type_code,
                    event_data1,
                    event_data2,
                    event_data3);

            if (event_offset_type != IPMI_MONITORING_EVENT_OFFSET_TYPE_UNKNOWN)
                printf (", %Xh", event_offset);
            else
                printf (", N/A");

            if (event_offset_type != IPMI_MONITORING_EVENT_OFFSET_TYPE_UNKNOWN)
                printf (", %s", event_offset_string);
            else
                printf (", N/A");
        }
        else if (record_type_class == IPMI_MONITORING_SEL_RECORD_TYPE_CLASS_TIMESTAMPED_OEM_RECORD
                 || record_type_class == IPMI_MONITORING_SEL_RECORD_TYPE_CLASS_NON_TIMESTAMPED_OEM_RECORD)
        {
            if (record_type_class == IPMI_MONITORING_SEL_RECORD_TYPE_CLASS_TIMESTAMPED_OEM_RECORD)
            {
                if ((manufacturer_id = ipmi_monitoring_sel_read_manufacturer_id (ctx)) < 0)
                {
                    collector_error( "ipmi_monitoring_sel_read_manufacturer_id(): %s",
                                     ipmi_monitoring_ctx_errormsg (ctx));
                    goto cleanup;
                }

                printf (", Manufacturer ID = %Xh", manufacturer_id);
            }

            if ((oem_data_len = ipmi_monitoring_sel_read_oem_data (ctx, oem_data, 1024)) < 0)
            {
                collector_error( "ipmi_monitoring_sel_read_oem_data(): %s",
                                 ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            printf (", OEM Data = ");

            for (j = 0; j < oem_data_len; j++)
                printf ("%02Xh ", oem_data[j]);
        }
        else
            printf (", N/A, N/A, N/A, N/A, N/A, N/A, N/A");

        printf ("\n");
#endif // NETDATA_COMMENTED
    }

    rv = 0;
    cleanup:
    if (ctx)
        ipmi_monitoring_ctx_destroy (ctx);
    return (rv);
}

// ----------------------------------------------------------------------------
// BEGIN NETDATA CODE

#define SENSORS_DICT_KEY_SIZE 2048
#define SPEED_TEST_ITERATIONS 5
#define IPMI_SENSORS_DASHBOARD_PRIORITY 90000
#define IPMI_SEL_DASHBOARD_PRIORITY 99000
#define IPMI_SENSORS_MIN_UPDATE_EVERY 5
#define IPMI_SEL_MIN_UPDATE_EVERY 60
#define IPMI_ENABLE_SEL_BY_DEFAULT true
#define IPMI_RESTART_EVERY_SECONDS 14400
#define IPMI_RESTART_IF_SENSORS_DONT_ITERATE_EVERY_SECONDS (10 * 60)

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

struct {
    const char *search;
    SIMPLE_PATTERN *pattern;
    const char *label;
} sensors_component_patterns[] = {

        // The order is important!
        // They are evaluated top to bottom
        // The first the matches is used

        {
                .search = "*DIMM*",
                .label = "Memory Module",
        },
        {
                .search = "FAN*|*_FAN*",
                .label = "Fan",
        },
        {
                .search = "*CPU*|SOC_*|*VDD*",
                .label = "Processor",
        },
        {
                .search = "IPU*",
                .label = "Image Processor",
        },
        {
                .search = "M2_*|*SSD*|*HSC*",
                .label = "Storage",
        },
        {
                .search = "MB_*",
                .label = "Motherboard",
        },
        {
                .search = "Watchdog|SEL|SYS_*",
                .label = "System",
        },
        {
                .search = "PS*|P_*",
                .label = "Power Supply",
        },
        {
                .search = "VR_P*",
                .label = "Processor",
        },
        {
                .search = NULL,
                .label = NULL,
        }
};

const char *collect_type_to_string(IPMI_COLLECTION_TYPE type) {
    if((type & (IPMI_COLLECT_TYPE_SENSORS|IPMI_COLLECT_TYPE_SEL)) == (IPMI_COLLECT_TYPE_SENSORS|IPMI_COLLECT_TYPE_SEL))
        return "sensors,sel";
    if(type & IPMI_COLLECT_TYPE_SEL)
        return "sel";
    if(type & IPMI_COLLECT_TYPE_SENSORS)
        return "sensors";

    return "unknown";
}

int netdata_ipmi_collect_data(struct ipmi_monitoring_ipmi_config *ipmi_config, IPMI_COLLECTION_TYPE type, struct netdata_ipmi_state *state) {
    errno = 0;

    if(type & IPMI_COLLECT_TYPE_SENSORS) {
        state->sensors.collected = 0;
        state->sensors.now_ut = now_monotonic_usec();

        if (_ipmimonitoring_sensors(ipmi_config, state) < 0) return -1;
    }

    if(type & IPMI_COLLECT_TYPE_SEL) {
        state->sel.events = 0;
        state->sel.now_ut = now_monotonic_usec();
        if(_ipmimonitoring_sel(ipmi_config, state) < 0) return -2;
    }

    return 0;
}

int netdata_ipmi_detect_speed_secs(struct ipmi_monitoring_ipmi_config *ipmi_config, IPMI_COLLECTION_TYPE type, struct netdata_ipmi_state *state) {
    int i, checks = SPEED_TEST_ITERATIONS, successful = 0;
    usec_t total = 0;

    for(i = 0 ; i < checks ; i++) {
        if(state->debug)
            fprintf(stderr, "%s: checking %s data collection speed iteration %d of %d\n",
                    program_name, collect_type_to_string(type), i+1, checks);

        // measure the time a data collection needs
        usec_t start = now_realtime_usec();

        if(netdata_ipmi_collect_data(ipmi_config, type, state) < 0)
            continue;

        usec_t end = now_realtime_usec();

        successful++;

        if(state->debug)
            fprintf(stderr, "%s: %s data collection speed was %llu usec\n",
                    program_name, collect_type_to_string(type), end - start);

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

int parse_inband_driver_type (const char *str)
{
    fatal_assert(str);

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

    return (-1);
}

int parse_outofband_driver_type (const char *str)
{
    fatal_assert(str);

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

static inline bool is_sensor_updated(usec_t last_collected_ut, usec_t now_ut, usec_t freq) {
    return (now_ut - last_collected_ut < freq * 2) ? true : false;
}

static const char *sensor_component(const char *sensor_name) {
    for(int i = 0; sensors_component_patterns[i].search ;i++) {
        if(!sensors_component_patterns[i].pattern)
            sensors_component_patterns[i].pattern = simple_pattern_create(sensors_component_patterns[i].search, "|", SIMPLE_PATTERN_EXACT, false);

        if(simple_pattern_matches(sensors_component_patterns[i].pattern, sensor_name))
            return sensors_component_patterns[i].label;
    }

    return "Other";
}

static size_t send_sensor_metrics_to_netdata(struct netdata_ipmi_state *state) {
    if(state->sensors.status != ICS_RUNNING) {
        if(state->debug)
            fprintf(stderr, "%s: %s() sensors state is not RUNNING\n",
                    program_name, __FUNCTION__ );
        return 0;
    }

    size_t total_sensors_sent = 0;
    int update_every = (int)(state->sensors.freq_ut / USEC_PER_SEC);
    struct sensor *sn;

    // generate the CHART/DIMENSION lines, if we have to
    dfe_start_reentrant(state->sensors.dict, sn) {
        if(!sn->do_metric && !sn->do_state)
            continue;

        bool did_metric = false, did_state = false;

        if(sn->do_metric) {
            if(!is_sensor_updated(sn->last_collected_metric_ut, state->updates.now_ut, state->sensors.freq_ut)) {
                if (state->debug)
                    fprintf(stderr, "%s: %s() sensor '%s' metric is not UPDATED (last updated %llu, now %llu, freq %llu\n",
                            program_name, __FUNCTION__, sn->sensor_name, sn->last_collected_metric_ut, state->updates.now_ut, state->sensors.freq_ut);
            }
            else {
                if (!sn->metric_chart_sent) {
                    sn->metric_chart_sent = true;

                    printf("CHART '%s_%s' '' '%s' '%s' '%s' '%s' '%s' %d %d\n",
                           sn->context, sn_dfe.name, sn->title, sn->units, sn->family, sn->context,
                           sn->chart_type, sn->priority + 1, update_every);

                    printf("CLABEL 'sensor' '%s' 1\n", sn->sensor_name);
                    printf("CLABEL 'type' '%s' 1\n", sn->type);
                    printf("CLABEL 'component' '%s' 1\n", sn->component);
                    printf("CLABEL_COMMIT\n");

                    printf("DIMENSION '%s' '' absolute 1 %d\n", sn->dimension, sn->multiplier);
                }

                printf("BEGIN '%s_%s'\n", sn->context, sn_dfe.name);

                switch (sn->sensor_reading_type) {
                    case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL:
                        printf("SET '%s' = %u\n", sn->dimension, sn->sensor_reading.bool_value
                        );
                        break;

                    case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32:
                        printf("SET '%s' = %u\n", sn->dimension, sn->sensor_reading.uint32_value
                        );
                        break;

                    case IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE:
                        printf("SET '%s' = %lld\n", sn->dimension,
                               (long long int) (sn->sensor_reading.double_value * sn->multiplier)
                        );
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

        if(sn->do_state) {
            if(!is_sensor_updated(sn->last_collected_state_ut, state->updates.now_ut, state->sensors.freq_ut)) {
                if (state->debug)
                    fprintf(stderr, "%s: %s() sensor '%s' state is not UPDATED (last updated %llu, now %llu, freq %llu\n",
                            program_name, __FUNCTION__, sn->sensor_name, sn->last_collected_state_ut, state->updates.now_ut, state->sensors.freq_ut);
            }
            else {
                if (!sn->state_chart_sent) {
                    sn->state_chart_sent = true;

                    printf("CHART 'ipmi.sensor_state_%s' '' 'IPMI Sensor State' 'state' 'states' 'ipmi.sensor_state' 'line' %d %d\n",
                           sn_dfe.name, sn->priority, update_every);

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

        if(did_metric || did_state)
            total_sensors_sent++;
    }
    dfe_done(sn);

    return total_sensors_sent;
}

static size_t send_sel_metrics_to_netdata(struct netdata_ipmi_state *state) {
    static int sel_chart_generated = 0;

    if(state->sel.status == ICS_RUNNING) {
        if(!sel_chart_generated) {
            sel_chart_generated = 1;
            printf("CHART ipmi.events '' 'IPMI Events' 'events' 'events' ipmi.sel area %d %d\n"
                    , state->sel.priority + 2
                    , (int)(state->sel.freq_ut / USEC_PER_SEC)
            );
            printf("DIMENSION events '' absolute 1 1\n");
        }

        printf(
                "BEGIN ipmi.events\n"
                "SET events = %zu\n"
                "END\n"
                , state->sel.events
        );
    }

    return state->sel.events;
}

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

static void sensor_set_value(struct sensor *sn, void *sensor_reading, struct netdata_ipmi_state *state) {
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

static void netdata_get_sensor(
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
        , struct netdata_ipmi_state *state
) {
    if(!sensor_name || !*sensor_name)
        sensor_name = "UNNAMED";

    state->sensors.collected++;

    char key[SENSORS_DICT_KEY_SIZE + 1];
    snprintfz(key, SENSORS_DICT_KEY_SIZE, "i%d_n%d_t%d_u%d_%s",
              record_id, sensor_number, sensor_reading_type, sensor_units, sensor_name);

    // find the sensor record
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(state->sensors.dict, key);
    if(likely(item)) {
        // recurring collection

        if(state->debug)
            fprintf(stderr, "Reusing sensor record for sensor '%s', id %d, number %d, type %d, state %d, units %d, reading_type %d\n",
                    sensor_name, record_id, sensor_number, sensor_type, sensor_state, sensor_units, sensor_reading_type);

        struct sensor *sn = dictionary_acquired_item_value(item);

        if(sensor_reading) {
            sensor_set_value(sn, sensor_reading, state);
            sn->last_collected_metric_ut = state->sensors.now_ut;
        }

        sn->last_collected_state_ut = state->sensors.now_ut;

        dictionary_acquired_item_release(state->sensors.dict, item);

        return;
    }

    if(state->debug)
        fprintf(stderr, "Allocating new sensor data record for sensor '%s', id %d, number %d, type %d, state %d, units %d, reading_type %d\n",
                sensor_name, record_id, sensor_number, sensor_type, sensor_state, sensor_units, sensor_reading_type);

    // check if it is excluded
    bool excluded_metric = excluded_record_ids_check(record_id);
    bool excluded_state = excluded_status_record_ids_check(record_id);

    if(excluded_metric) {
        if(state->debug)
            fprintf(stderr, "Sensor '%s' is excluded by excluded_record_ids_check()\n", sensor_name);
    }

    if(excluded_state) {
        if(state->debug)
            fprintf(stderr, "Sensor '%s' is excluded for status check, by excluded_status_record_ids_check()\n", sensor_name);
    }

    struct sensor t = {
            .sensor_type = sensor_type,
            .sensor_state = sensor_state,
            .sensor_units = sensor_units,
            .sensor_reading_type = sensor_reading_type,
            .sensor_name = strdupz(sensor_name ? sensor_name : ""),
            .component = sensor_component(sensor_name),
            .type = _get_sensor_type_string (sensor_type),
            .do_state = !excluded_state,
            .do_metric = !excluded_metric,
    };

    switch(sensor_units) {
        case IPMI_MONITORING_SENSOR_UNITS_CELSIUS:
            t.dimension = "temperature";
            t.context = "ipmi.sensor_temperature_c";
            t.title = "IPMI Sensor Temperature Celsius";
            t.units = "Celsius";
            t.family = "temperatures";
            t.chart_type = "line";
            t.priority = state->sensors.priority + 10;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_FAHRENHEIT:
            t.dimension = "temperature";
            t.context = "ipmi.sensor_temperature_f";
            t.title = "IPMI Sensor Temperature Fahrenheit";
            t.units = "Fahrenheit";
            t.family = "temperatures";
            t.chart_type = "line";
            t.priority = state->sensors.priority + 20;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_VOLTS:
            t.dimension = "voltage";
            t.context = "ipmi.sensor_voltage";
            t.title = "IPMI Sensor Voltage";
            t.units = "Volts";
            t.family = "voltages";
            t.chart_type = "line";
            t.priority = state->sensors.priority + 30;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_AMPS:
            t.dimension = "ampere";
            t.context = "ipmi.sensor_ampere";
            t.title = "IPMI Sensor Current";
            t.units = "Amps";
            t.family = "current";
            t.chart_type = "line";
            t.priority = state->sensors.priority + 40;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_RPM:
            t.dimension = "rotations";
            t.context = "ipmi.fan_speed";
            t.title = "IPMI Sensor Fans Speed";
            t.units = "RPM";
            t.family = "fans";
            t.chart_type = "line";
            t.priority = state->sensors.priority + 50;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_WATTS:
            t.dimension = "power";
            t.context = "ipmi.sensor_power";
            t.title = "IPMI Sensor Power";
            t.units = "Watts";
            t.family = "power";
            t.chart_type = "line";
            t.priority = state->sensors.priority + 60;
            break;

        case IPMI_MONITORING_SENSOR_UNITS_PERCENT:
            t.dimension = "percentage";
            t.context = "ipmi.sensor_percent";
            t.title = "IPMI Sensor Reading Percentage";
            t.units = "%%";
            t.family = "other";
            t.chart_type = "line";
            t.priority = state->sensors.priority + 70;
            break;

        default:
            t.priority = state->sensors.priority + 80;
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
        sensor_set_value(&t, sensor_reading, state);
        t.last_collected_metric_ut = state->sensors.now_ut;
    }
    t.last_collected_state_ut = state->sensors.now_ut;

    dictionary_set(state->sensors.dict, key, &t, sizeof(t));
}

static void netdata_get_sel(int record_id, int record_type_class, int sel_state, struct netdata_ipmi_state *state) {
    (void)record_id;
    (void)record_type_class;
    (void)sel_state;

    state->sel.events++;
}

int host_is_local(const char *host) {
    if (host && (!strcmp(host, "localhost") || !strcmp(host, "127.0.0.1") || !strcmp(host, "::1")))
        return (1);

    return (0);
}

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

    if(t->debug) fprintf(stderr, "%s: calling _init_ipmi_config() for %s\n",
                      program_name, collect_type_to_string(t->type));

    _init_ipmi_config(&t->ipmi_config);

    if(t->debug) fprintf(stderr, "%s: detecting IPMI minimum update frequency for %s...\n",
                      program_name, collect_type_to_string(t->type));

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

    freq_s = t->freq_s = MAX(t->freq_s, freq_s);

    if(t->debug) {
        fprintf(stderr, "%s: IPMI minimum update frequency of %s was calculated to %d seconds.\n",
                program_name, collect_type_to_string(t->type), t->freq_s);

        fprintf(stderr, "%s: starting data collection of %s\n",
                program_name, collect_type_to_string(t->type));
    }

    size_t iteration = 0, failures = 0;
    usec_t step = t->freq_s * USEC_PER_SEC;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1 ; iteration++) {
        heartbeat_next(&hb, step);

        if(t->debug)
            fprintf(stderr, "%s: calling netdata_ipmi_collect_data() for %s\n",
                    program_name, collect_type_to_string(t->type));

        struct netdata_ipmi_state tmp_state = t->state;
        tmp_state.debug = true;

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
                  __FUNCTION__, collect_type_to_string(t->type), failures, iteration);

            if(t->type & IPMI_COLLECT_TYPE_SENSORS) {
                t->state.sensors.status = ICS_FAILED;
                t->state.sensors.last_iteration_ut = 0;
            }

            if(t->type & IPMI_COLLECT_TYPE_SEL) {
                t->state.sel.status = ICS_FAILED;
                t->state.sel.last_iteration_ut = 0;
            }

            return ptr;
        }

        spinlock_lock(&t->spinlock);
        t->state = tmp_state;
        spinlock_unlock(&t->spinlock);
    }

    return ptr;
}

int main (int argc, char **argv) {
    bool netdata_do_sel = IPMI_ENABLE_SEL_BY_DEFAULT;

    stderror = stderr;
    clocks_init();

    int update_every = IPMI_SENSORS_MIN_UPDATE_EVERY; // this is the minimum update frequency
    int update_every_sel = IPMI_SEL_MIN_UPDATE_EVERY; // this is the minimum update frequency for SEL events
    bool debug = false;

    // ------------------------------------------------------------------------
    // initialization of netdata plugin

    program_name = "freeipmi.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;


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
            printf("%s %s\n", program_name, VERSION);
            exit(0);
        }
        else if(strcmp("debug", argv[i]) == 0) {
            debug = true;
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
        else if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata %s %s\n"
                    " Copyright (C) 2023 Netdata Inc.\n"
                    " Released under GNU General Public License v3 or later.\n"
                    " All rights reserved.\n"
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
                    "  hostname HOST\n"
                    "  username USER\n"
                    "  password PASS           connect to remote IPMI host\n"
                    "                          default: local IPMI processor\n"
                    "\n"
                    "  noauthcodecheck         don't check the authentication codes returned\n"
                    "\n"
                    " driver-type IPMIDRIVER\n"
                    "                          Specify the driver type to use instead of doing an auto selection. \n"
                    "                          The currently available outofband drivers are LAN and  LAN_2_0,\n"
                    "                          which  perform  IPMI  1.5  and  IPMI  2.0 respectively. \n"
                    "                          The currently available inband drivers are KCS, SSIF, OPENIPMI and SUNBMC.\n"
                    "\n"
                    "  sdr-cache-dir PATH      directory for SDR cache files\n"
                    "                          default: %s\n"
                    "\n"
                    "  sensor-config-file FILE filename to read sensor configuration\n"
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
                    " https://github.com/netdata/netdata/tree/master/collectors/freeipmi.plugin\n"
                    "\n"
                    , program_name, VERSION
                    , update_every
                    , netdata_do_sel?"enabled":"disabled"
                    , sdr_cache_directory?sdr_cache_directory:"system default"
                    , sensor_config_file?sensor_config_file:"system default"
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
                protocol_version=parse_outofband_driver_type(argv[++i]);
                if(debug) fprintf(stderr, "%s: outband protocol version set to '%d'\n",
                                  program_name, protocol_version);
            }
            else {
                driver_type=parse_inband_driver_type(argv[++i]);
                if(debug) fprintf(stderr, "%s: inband driver type set to '%d'\n",
                                  program_name, driver_type);
            }
            continue;
        } else if (i < argc && strcmp("noauthcodecheck", argv[i]) == 0) {
            if (!hostname || host_is_local(hostname)) {
                if (debug)
                    fprintf(stderr, "%s: noauthcodecheck workaround flag is ignored for inband configuration\n",
                            program_name);

            }
            else if (protocol_version < 0 || protocol_version == IPMI_MONITORING_PROTOCOL_VERSION_1_5) {
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

    errno = 0;

    if(freq_s < update_every)
        collector_error("%s(): update frequency %d seconds is too small for IPMI. Using %d.",
                        __FUNCTION__, freq_s, update_every);

    freq_s = update_every = MAX(freq_s, update_every);
    update_every_sel = MAX(update_every, update_every_sel);

    // ------------------------------------------------------------------------
    // initialize IPMI

    if(debug) {
        fprintf(stderr, "%s: calling ipmi_monitoring_init()\n", program_name);
        //ipmimonitoring_init_flags|=IPMI_MONITORING_FLAGS_DEBUG|IPMI_MONITORING_FLAGS_DEBUG_IPMI_PACKETS;
    }

    if(ipmi_monitoring_init(ipmimonitoring_init_flags, &errnum) < 0)
        fatal("ipmi_monitoring_init: %s", ipmi_monitoring_ctx_strerror(errnum));

    // ------------------------------------------------------------------------
    // create the data collection threads

    struct ipmi_collection_thread sensors_data = {
            .type = IPMI_COLLECT_TYPE_SENSORS,
            .freq_s = update_every,
            .spinlock = NETDATA_SPINLOCK_INITIALIZER,
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
            .spinlock = NETDATA_SPINLOCK_INITIALIZER,
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

    netdata_thread_t sensors_thread = 0, sel_thread = 0;

    netdata_thread_create(&sensors_thread, "IPMI[sensors]", NETDATA_THREAD_OPTION_DONT_LOG, netdata_ipmi_collection_thread, &sensors_data);

    if(netdata_do_sel)
        netdata_thread_create(&sel_thread, "IPMI[sel]", NETDATA_THREAD_OPTION_DONT_LOG, netdata_ipmi_collection_thread, &sel_data);

    // ------------------------------------------------------------------------
    // the main loop

    if(debug) fprintf(stderr, "%s: starting data collection\n", program_name);

    time_t started_t = now_monotonic_sec();

    size_t iteration = 0;
    usec_t step = 100 * USEC_PER_MS;
    bool global_chart_created = false;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1 ; iteration++) {
        usec_t dt = heartbeat_next(&hb, step);

        struct netdata_ipmi_state state = {0 };

        spinlock_lock(&sensors_data.spinlock);
        state.sensors = sensors_data.state.sensors;
        spinlock_unlock(&sensors_data.spinlock);

        spinlock_lock(&sel_data.spinlock);
        state.sel = sel_data.state.sel;
        spinlock_unlock(&sel_data.spinlock);

        switch(state.sensors.status) {
            case ICS_RUNNING:
                step = update_every * USEC_PER_SEC;
                if(state.sensors.last_iteration_ut < now_monotonic_usec() - IPMI_RESTART_IF_SENSORS_DONT_ITERATE_EVERY_SECONDS * USEC_PER_SEC) {
                    collector_error("%s(): sensors have not be collected for %zu seconds. Exiting to restart.",
                                    __FUNCTION__, (size_t)((now_monotonic_usec() - state.sensors.last_iteration_ut) / USEC_PER_SEC));

                    fprintf(stdout, "EXIT\n");
                    fflush(stdout);
                    exit(0);
                }
                break;

            case ICS_INIT:
                continue;

            case ICS_INIT_FAILED:
                collector_error("%s(): sensors failed to initialize. Calling DISABLE.", __FUNCTION__);
                fprintf(stdout, "DISABLE\n");
                fflush(stdout);
                exit(0);
                break;

            case ICS_FAILED:
                collector_error("%s(): sensors fails repeatedly to collect metrics. Exiting to restart.", __FUNCTION__);
                fprintf(stdout, "EXIT\n");
                fflush(stdout);
                exit(0);
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

        if(debug) fprintf(stderr, "%s: calling send_sensor_metrics_to_netdata()\n", program_name);

        state.updates.now_ut = now_monotonic_usec();
        send_sensor_metrics_to_netdata(&state);

        if(netdata_do_sel)
            send_sel_metrics_to_netdata(&state);

        if(debug)
            fprintf(stderr, "%s: iteration %zu, dt %llu usec, sensors ever collected %zu, sensors last collected %zu \n"
                    , program_name
                    , iteration
                    , dt
                    , dictionary_entries(state.sensors.dict)
                    , state.sensors.collected
            );

        if (!global_chart_created) {
            global_chart_created = true;

            fprintf(stdout,
                    "CHART netdata.freeipmi_availability_status '' 'Plugin availability status' 'status' "
                    "plugins netdata.plugin_availability_status line 146000 %d\n"
                    "DIMENSION available '' absolute 1 1\n",
                    update_every);
        }
        fprintf(stdout,
                "BEGIN netdata.freeipmi_availability_status\n"
                "SET available = 1\n"
                "END\n");

        // restart check (14400 seconds)
        if (now_monotonic_sec() - started_t > IPMI_RESTART_EVERY_SECONDS) {
            collector_error("%s(): reached my lifetime expectancy. Exiting to restart.", __FUNCTION__);
            fprintf(stdout, "EXIT\n");
            fflush(stdout);
            exit(0);
        }

        fflush(stdout);
    }
}
