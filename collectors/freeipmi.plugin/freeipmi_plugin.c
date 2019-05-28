// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata freeipmi.plugin
 *  Copyright (C) 2017 Costa Tsaousis
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

#include "../../libnetdata/libnetdata.h"

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <assert.h>
#include <errno.h>
#include <unistd.h>
#include <sys/time.h>

#ifdef HAVE_FREEIPMI

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

// ----------------------------------------------------------------------------

// callback required by fatal()
void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

void send_statistics( const char *action, const char *action_result, const char *action_data) {
    (void)action;
    (void)action_result;
    (void)action_data;
    return;
}

// callbacks required by popen()
void signals_block(void) {};
void signals_unblock(void) {};
void signals_reset(void) {};

// callback required by eval()
int health_variable_lookup(const char *variable, uint32_t hash, struct rrdcalc *rc, calculated_number *result) {
    (void)variable;
    (void)hash;
    (void)rc;
    (void)result;
    return 0;
};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// ----------------------------------------------------------------------------

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
unsigned char *ipmi_k_g = NULL;
unsigned int ipmi_k_g_len = 0;
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
    assert (ipmi_config);

    ipmi_config->driver_type = driver_type;
    ipmi_config->disable_auto_probe = disable_auto_probe;
    ipmi_config->driver_address = driver_address;
    ipmi_config->register_spacing = register_spacing;
    ipmi_config->driver_device = driver_device;

    ipmi_config->protocol_version = protocol_version;
    ipmi_config->username = username;
    ipmi_config->password = password;
    ipmi_config->k_g = ipmi_k_g;
    ipmi_config->k_g_len = ipmi_k_g_len;
    ipmi_config->privilege_level = privilege_level;
    ipmi_config->authentication_type = authentication_type;
    ipmi_config->cipher_suite_id = cipher_suite_id;
    ipmi_config->session_timeout_len = session_timeout;
    ipmi_config->retransmission_timeout_len = retransmission_timeout;

    ipmi_config->workaround_flags = workaround_flags;
}

#ifdef NETDATA_COMMENTED
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
#endif // NETDATA_COMMENTED


// ----------------------------------------------------------------------------
// BEGIN NETDATA CODE

static int debug = 0;

static int netdata_update_every = 5; // this is the minimum update frequency
static int netdata_priority = 90000;
static int netdata_do_sel = 1;

static size_t netdata_sensors_updated = 0;
static size_t netdata_sensors_collected = 0;
static size_t netdata_sel_events = 0;
static size_t netdata_sensors_states_nominal = 0;
static size_t netdata_sensors_states_warning = 0;
static size_t netdata_sensors_states_critical = 0;

struct sensor {
    int record_id;
    int sensor_number;
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

    int sent;
    int ignore;
    int exposed;
    int updated;
    struct sensor *next;
} *sensors_root = NULL;

static void netdata_mark_as_not_updated() {
    struct sensor *sn;
    for(sn = sensors_root; sn ;sn = sn->next)
        sn->updated = sn->sent = 0;

    netdata_sensors_updated = 0;
    netdata_sensors_collected = 0;
    netdata_sel_events = 0;

    netdata_sensors_states_nominal = 0;
    netdata_sensors_states_warning = 0;
    netdata_sensors_states_critical = 0;
}

static void send_chart_to_netdata_for_units(int units) {
    struct sensor *sn;

    switch(units) {
        case IPMI_MONITORING_SENSOR_UNITS_CELSIUS:
            printf("CHART ipmi.temperatures_c '' 'System Celsius Temperatures read by IPMI' 'Celsius' 'temperatures' 'ipmi.temperatures_c' 'line' %d %d\n"
                   , netdata_priority + 10
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_FAHRENHEIT:
            printf("CHART ipmi.temperatures_f '' 'System Fahrenheit Temperatures read by IPMI' 'Fahrenheit' 'temperatures' 'ipmi.temperatures_f' 'line' %d %d\n"
                   , netdata_priority + 11
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_VOLTS:
            printf("CHART ipmi.volts '' 'System Voltages read by IPMI' 'Volts' 'voltages' 'ipmi.voltages' 'line' %d %d\n"
                   , netdata_priority + 12
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_AMPS:
            printf("CHART ipmi.amps '' 'System Current read by IPMI' 'Amps' 'current' 'ipmi.amps' 'line' %d %d\n"
                   , netdata_priority + 13
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_RPM:
            printf("CHART ipmi.rpm '' 'System Fans read by IPMI' 'RPM' 'fans' 'ipmi.rpm' 'line' %d %d\n"
                   , netdata_priority + 14
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_WATTS:
            printf("CHART ipmi.watts '' 'System Power read by IPMI' 'Watts' 'power' 'ipmi.watts' 'line' %d %d\n"
                   , netdata_priority + 5
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_PERCENT:
            printf("CHART ipmi.percent '' 'System Metrics read by IPMI' '%%' 'other' 'ipmi.percent' 'line' %d %d\n"
                   , netdata_priority + 15
                   , netdata_update_every
            );
            break;

        default:
            for(sn = sensors_root; sn; sn = sn->next)
                if(sn->sensor_units == units)
                    sn->ignore = 1;
            return;
    }

    for(sn = sensors_root; sn; sn = sn->next) {
        if(sn->sensor_units == units && sn->updated && !sn->ignore) {
            sn->exposed = 1;

            switch(sn->sensor_reading_type) {
                case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL:
                case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32:
                    printf("DIMENSION i%d_n%d_r%d '%s i%d' absolute 1 1\n"
                           , sn->sensor_number
                           , sn->record_id
                           , sn->sensor_reading_type
                           , sn->sensor_name
                           , sn->sensor_number
                    );
                    break;

                case IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE:
                    printf("DIMENSION i%d_n%d_r%d '%s i%d' absolute 1 1000\n"
                           , sn->sensor_number
                           , sn->record_id
                           , sn->sensor_reading_type
                           , sn->sensor_name
                           , sn->sensor_number
                    );
                    break;

                default:
                    sn->ignore = 1;
                    break;
            }
        }
    }
}

static void send_metrics_to_netdata_for_units(int units) {
    struct sensor *sn;

    switch(units) {
        case IPMI_MONITORING_SENSOR_UNITS_CELSIUS:
            printf("BEGIN ipmi.temperatures_c\n");
            break;

        case IPMI_MONITORING_SENSOR_UNITS_FAHRENHEIT:
            printf("BEGIN ipmi.temperatures_f\n");
            break;

        case IPMI_MONITORING_SENSOR_UNITS_VOLTS:
            printf("BEGIN ipmi.volts\n");
            break;

        case IPMI_MONITORING_SENSOR_UNITS_AMPS:
            printf("BEGIN ipmi.amps\n");
            break;

        case IPMI_MONITORING_SENSOR_UNITS_RPM:
            printf("BEGIN ipmi.rpm\n");
            break;

        case IPMI_MONITORING_SENSOR_UNITS_WATTS:
            printf("BEGIN ipmi.watts\n");
            break;

        case IPMI_MONITORING_SENSOR_UNITS_PERCENT:
            printf("BEGIN ipmi.percent\n");
            break;

        default:
            for(sn = sensors_root; sn; sn = sn->next)
                if(sn->sensor_units == units)
                    sn->ignore = 1;
            return;
    }

    for(sn = sensors_root; sn; sn = sn->next) {
        if(sn->sensor_units == units && sn->updated && !sn->sent && !sn->ignore) {
            netdata_sensors_updated++;

            sn->sent = 1;

            switch(sn->sensor_reading_type) {
                case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL:
                    printf("SET i%d_n%d_r%d = %u\n"
                           , sn->sensor_number
                           , sn->record_id
                           , sn->sensor_reading_type
                           , sn->sensor_reading.bool_value
                    );
                    break;

                case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32:
                    printf("SET i%d_n%d_r%d = %u\n"
                           , sn->sensor_number
                           , sn->record_id
                           , sn->sensor_reading_type
                           , sn->sensor_reading.uint32_value
                    );
                    break;

                case IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE:
                    printf("SET i%d_n%d_r%d = %lld\n"
                           , sn->sensor_number
                           , sn->record_id
                           , sn->sensor_reading_type
                           , (long long int)(sn->sensor_reading.double_value * 1000)
                    );
                    break;

                default:
                    sn->ignore = 1;
                    break;
            }
        }
    }

    printf("END\n");
}

static void send_metrics_to_netdata() {
    static int sel_chart_generated = 0, sensors_states_chart_generated = 0;
    struct sensor *sn;

    if(netdata_do_sel && !sel_chart_generated) {
        sel_chart_generated = 1;
        printf("CHART ipmi.events '' 'IPMI Events' 'events' 'events' ipmi.sel area %d %d\n"
               , netdata_priority + 2
               , netdata_update_every
        );
        printf("DIMENSION events '' absolute 1 1\n");
    }

    if(!sensors_states_chart_generated) {
        sensors_states_chart_generated = 1;
        printf("CHART ipmi.sensors_states '' 'IPMI Sensors State' 'sensors' 'states' ipmi.sensors_states line %d %d\n"
               , netdata_priority + 1
               , netdata_update_every
        );
        printf("DIMENSION nominal '' absolute 1 1\n");
        printf("DIMENSION critical '' absolute 1 1\n");
        printf("DIMENSION warning '' absolute 1 1\n");
    }

    // generate the CHART/DIMENSION lines, if we have to
    for(sn = sensors_root; sn; sn = sn->next)
        if(sn->updated && !sn->exposed && !sn->ignore)
            send_chart_to_netdata_for_units(sn->sensor_units);

    if(netdata_do_sel) {
        printf(
                "BEGIN ipmi.events\n"
                "SET events = %zu\n"
                "END\n"
                , netdata_sel_events
        );
    }

    printf(
           "BEGIN ipmi.sensors_states\n"
           "SET nominal = %zu\n"
           "SET warning = %zu\n"
           "SET critical = %zu\n"
           "END\n"
           , netdata_sensors_states_nominal
           , netdata_sensors_states_warning
           , netdata_sensors_states_critical
    );

    // send metrics to netdata
    for(sn = sensors_root; sn; sn = sn->next)
        if(sn->updated && sn->exposed && !sn->sent && !sn->ignore)
            send_metrics_to_netdata_for_units(sn->sensor_units);

}

static int *excluded_record_ids = NULL;
size_t excluded_record_ids_length = 0;

static void excluded_record_ids_parse(const char *s) {
    if(!s) return;

    while(*s) {
        while(*s && !isdigit(*s)) s++;

        if(isdigit(*s)) {
            char *e;
            unsigned long n = strtoul(s, &e, 10);
            s = e;

            if(n != 0) {
                excluded_record_ids = realloc(excluded_record_ids, (excluded_record_ids_length + 1) * sizeof(int));
                if(!excluded_record_ids) {
                    fprintf(stderr, "freeipmi.plugin: failed to allocate memory. Exiting.");
                    exit(1);
                }
                excluded_record_ids[excluded_record_ids_length++] = (int)n;
            }
        }
    }

    if(debug) {
        fprintf(stderr, "freeipmi.plugin: excluded record ids:");
        size_t i;
        for(i = 0; i < excluded_record_ids_length; i++) {
            fprintf(stderr, " %d", excluded_record_ids[i]);
        }
        fprintf(stderr, "\n");
    }
}

static int *excluded_status_record_ids = NULL;
size_t excluded_status_record_ids_length = 0;

static void excluded_status_record_ids_parse(const char *s) {
    if(!s) return;

    while(*s) {
        while(*s && !isdigit(*s)) s++;

        if(isdigit(*s)) {
            char *e;
            unsigned long n = strtoul(s, &e, 10);
            s = e;

            if(n != 0) {
                excluded_status_record_ids = realloc(excluded_status_record_ids, (excluded_status_record_ids_length + 1) * sizeof(int));
                if(!excluded_status_record_ids) {
                    fprintf(stderr, "freeipmi.plugin: failed to allocate memory. Exiting.");
                    exit(1);
                }
                excluded_status_record_ids[excluded_status_record_ids_length++] = (int)n;
            }
        }
    }

    if(debug) {
        fprintf(stderr, "freeipmi.plugin: excluded status record ids:");
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

static void netdata_get_sensor(
          int record_id
        , int sensor_number
        , int sensor_type
        , int sensor_state
        , int sensor_units
        , int sensor_reading_type
        , char *sensor_name
        , void *sensor_reading
) {
    // find the sensor record
    struct sensor *sn;
    for(sn = sensors_root; sn ;sn = sn->next)
        if(     sn->record_id           == record_id &&
                sn->sensor_number       == sensor_number &&
                sn->sensor_reading_type == sensor_reading_type &&
                sn->sensor_units        == sensor_units &&
                !strcmp(sn->sensor_name, sensor_name)
                )
            break;

    if(!sn) {
        // not found, create it
        // check if it is excluded
        if(excluded_record_ids_check(record_id)) {
            if(debug) fprintf(stderr, "Sensor '%s' is excluded by excluded_record_ids_check()\n", sensor_name);
            return;
        }

        if(debug) fprintf(stderr, "Allocating new sensor data record for sensor '%s', id %d, number %d, type %d, state %d, units %d, reading_type %d\n", sensor_name, record_id, sensor_number, sensor_type, sensor_state, sensor_units, sensor_reading_type);

        sn = calloc(1, sizeof(struct sensor));
        if(!sn) {
            fatal("cannot allocate %zu bytes of memory.", sizeof(struct sensor));
        }

        sn->record_id = record_id;
        sn->sensor_number = sensor_number;
        sn->sensor_type = sensor_type;
        sn->sensor_state = sensor_state;
        sn->sensor_units = sensor_units;
        sn->sensor_reading_type = sensor_reading_type;
        sn->sensor_name = strdup(sensor_name);
        if(!sn->sensor_name) {
            fatal("cannot allocate %zu bytes of memory.", strlen(sensor_name));
        }

        sn->next = sensors_root;
        sensors_root = sn;
    }
    else {
        if(debug) fprintf(stderr, "Reusing sensor record for sensor '%s', id %d, number %d, type %d, state %d, units %d, reading_type %d\n", sensor_name, record_id, sensor_number, sensor_type, sensor_state, sensor_units, sensor_reading_type);
    }

    switch(sensor_reading_type) {
        case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL:
            sn->sensor_reading.bool_value = *((uint8_t *)sensor_reading);
            sn->updated = 1;
            netdata_sensors_collected++;
            break;

        case IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32:
            sn->sensor_reading.uint32_value = *((uint32_t *)sensor_reading);
            sn->updated = 1;
            netdata_sensors_collected++;
            break;

        case IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE:
            sn->sensor_reading.double_value = *((double *)sensor_reading);
            sn->updated = 1;
            netdata_sensors_collected++;
            break;

        default:
            if(debug) fprintf(stderr, "Unknown reading type - Ignoring sensor record for sensor '%s', id %d, number %d, type %d, state %d, units %d, reading_type %d\n", sensor_name, record_id, sensor_number, sensor_type, sensor_state, sensor_units, sensor_reading_type);
            sn->ignore = 1;
            break;
    }

    // check if it is excluded
    if(excluded_status_record_ids_check(record_id)) {
        if(debug) fprintf(stderr, "Sensor '%s' is excluded for status check, by excluded_status_record_ids_check()\n", sensor_name);
        return;
    }

    switch(sensor_state) {
        case IPMI_MONITORING_STATE_NOMINAL:
            netdata_sensors_states_nominal++;
            break;

        case IPMI_MONITORING_STATE_WARNING:
            netdata_sensors_states_warning++;
            break;

        case IPMI_MONITORING_STATE_CRITICAL:
            netdata_sensors_states_critical++;
            break;

        default:
            break;
    }
}

static void netdata_get_sel(
          int record_id
        , int record_type_class
        , int sel_state
) {
    (void)record_id;
    (void)record_type_class;
    (void)sel_state;

    netdata_sel_events++;
}


// END NETDATA CODE
// ----------------------------------------------------------------------------


static int
_ipmimonitoring_sensors (struct ipmi_monitoring_ipmi_config *ipmi_config)
{
    ipmi_monitoring_ctx_t ctx = NULL;
    unsigned int sensor_reading_flags = 0;
    int i;
    int sensor_count;
    int rv = -1;

    if (!(ctx = ipmi_monitoring_ctx_create ())) {
        error("ipmi_monitoring_ctx_create()");
        goto cleanup;
    }

    if (sdr_cache_directory)
    {
        if (ipmi_monitoring_ctx_sdr_cache_directory (ctx,
                sdr_cache_directory) < 0)
        {
            error("ipmi_monitoring_ctx_sdr_cache_directory(): %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

    /* Must call otherwise only default interpretations ever used */
    if (sensor_config_file)
    {
        if (ipmi_monitoring_ctx_sensor_config_file (ctx,
                sensor_config_file) < 0)
        {
            error( "ipmi_monitoring_ctx_sensor_config_file(): %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else
    {
        if (ipmi_monitoring_ctx_sensor_config_file (ctx, NULL) < 0)
        {
            error( "ipmi_monitoring_ctx_sensor_config_file(): %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

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

    if (!record_ids_length && !sensor_types_length)
    {
        if ((sensor_count = ipmi_monitoring_sensor_readings_by_record_id (ctx,
                hostname,
                ipmi_config,
                sensor_reading_flags,
                NULL,
                0,
                NULL,
                NULL)) < 0)
        {
            error( "ipmi_monitoring_sensor_readings_by_record_id(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else if (record_ids_length)
    {
        if ((sensor_count = ipmi_monitoring_sensor_readings_by_record_id (ctx,
                hostname,
                ipmi_config,
                sensor_reading_flags,
                record_ids,
                record_ids_length,
                NULL,
                NULL)) < 0)
        {
            error( "ipmi_monitoring_sensor_readings_by_record_id(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else
    {
        if ((sensor_count = ipmi_monitoring_sensor_readings_by_sensor_type (ctx,
                hostname,
                ipmi_config,
                sensor_reading_flags,
                sensor_types,
                sensor_types_length,
                NULL,
                NULL)) < 0)
        {
            error( "ipmi_monitoring_sensor_readings_by_sensor_type(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }

#ifdef NETDATA_COMMENTED
    printf ("%s, %s, %s, %s, %s, %s, %s, %s, %s, %s\n",
            "Record ID",
            "Sensor Name",
            "Sensor Number",
            "Sensor Type",
            "Sensor State",
            "Sensor Reading",
            "Sensor Units",
            "Sensor Event/Reading Type Code",
            "Sensor Event Bitmask",
            "Sensor Event String");
#endif // NETDATA_COMMENTED

    for (i = 0; i < sensor_count; i++, ipmi_monitoring_sensor_iterator_next (ctx))
    {
        int record_id, sensor_number, sensor_type, sensor_state, sensor_units,
                sensor_reading_type;

#ifdef NETDATA_COMMENTED
        int sensor_bitmask_type, sensor_bitmask, event_reading_type_code;
        char **sensor_bitmask_strings = NULL;
        const char *sensor_type_str;
        const char *sensor_state_str;
#endif // NETDATA_COMMENTED

        char *sensor_name = NULL;
        void *sensor_reading;

        if ((record_id = ipmi_monitoring_sensor_read_record_id (ctx)) < 0)
        {
            error( "ipmi_monitoring_sensor_read_record_id(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sensor_number = ipmi_monitoring_sensor_read_sensor_number (ctx)) < 0)
        {
            error( "ipmi_monitoring_sensor_read_sensor_number(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sensor_type = ipmi_monitoring_sensor_read_sensor_type (ctx)) < 0)
        {
            error( "ipmi_monitoring_sensor_read_sensor_type(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if (!(sensor_name = ipmi_monitoring_sensor_read_sensor_name (ctx)))
        {
            error( "ipmi_monitoring_sensor_read_sensor_name(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sensor_state = ipmi_monitoring_sensor_read_sensor_state (ctx)) < 0)
        {
            error( "ipmi_monitoring_sensor_read_sensor_state(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sensor_units = ipmi_monitoring_sensor_read_sensor_units (ctx)) < 0)
        {
            error( "ipmi_monitoring_sensor_read_sensor_units(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

#ifdef NETDATA_COMMENTED
        if ((sensor_bitmask_type = ipmi_monitoring_sensor_read_sensor_bitmask_type (ctx)) < 0)
        {
            error( "ipmi_monitoring_sensor_read_sensor_bitmask_type(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
        if ((sensor_bitmask = ipmi_monitoring_sensor_read_sensor_bitmask (ctx)) < 0)
        {
            error(
                   "ipmi_monitoring_sensor_read_sensor_bitmask(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

         /* it's ok for this to be NULL, i.e. sensor_bitmask ==
          * IPMI_MONITORING_SENSOR_BITMASK_TYPE_UNKNOWN
          */
        sensor_bitmask_strings = ipmi_monitoring_sensor_read_sensor_bitmask_strings (ctx);
        
        
        
#endif // NETDATA_COMMENTED

        if ((sensor_reading_type = ipmi_monitoring_sensor_read_sensor_reading_type (ctx)) < 0)
        {
            error( "ipmi_monitoring_sensor_read_sensor_reading_type(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        sensor_reading = ipmi_monitoring_sensor_read_sensor_reading (ctx);

#ifdef NETDATA_COMMENTED
        if ((event_reading_type_code = ipmi_monitoring_sensor_read_event_reading_type_code (ctx)) < 0)
        {
            error( "ipmi_monitoring_sensor_read_event_reading_type_code(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
#endif // NETDATA_COMMENTED

        netdata_get_sensor(
                record_id
                , sensor_number
                , sensor_type
                , sensor_state
                , sensor_units
                , sensor_reading_type
                , sensor_name
                , sensor_reading
        );

#ifdef NETDATA_COMMENTED
        if (!strlen (sensor_name))
            sensor_name = "N/A";

        sensor_type_str = _get_sensor_type_string (sensor_type);

        printf ("%d, %s, %d, %s",
                record_id,
                sensor_name,
                sensor_number,
                sensor_type_str);

        if (sensor_state == IPMI_MONITORING_STATE_NOMINAL)
            sensor_state_str = "Nominal";
        else if (sensor_state == IPMI_MONITORING_STATE_WARNING)
            sensor_state_str = "Warning";
        else if (sensor_state == IPMI_MONITORING_STATE_CRITICAL)
            sensor_state_str = "Critical";
        else
            sensor_state_str = "N/A";

        printf (", %s", sensor_state_str);

        if (sensor_reading)
        {
            const char *sensor_units_str;

            if (sensor_reading_type == IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER8_BOOL)
                printf (", %s",
                        (*((uint8_t *)sensor_reading) ? "true" : "false"));
            else if (sensor_reading_type == IPMI_MONITORING_SENSOR_READING_TYPE_UNSIGNED_INTEGER32)
                printf (", %u",
                        *((uint32_t *)sensor_reading));
            else if (sensor_reading_type == IPMI_MONITORING_SENSOR_READING_TYPE_DOUBLE)
                printf (", %.2f",
                        *((double *)sensor_reading));
            else
                printf (", N/A");

            if (sensor_units == IPMI_MONITORING_SENSOR_UNITS_CELSIUS)
                sensor_units_str = "C";
            else if (sensor_units == IPMI_MONITORING_SENSOR_UNITS_FAHRENHEIT)
                sensor_units_str = "F";
            else if (sensor_units == IPMI_MONITORING_SENSOR_UNITS_VOLTS)
                sensor_units_str = "V";
            else if (sensor_units == IPMI_MONITORING_SENSOR_UNITS_AMPS)
                sensor_units_str = "A";
            else if (sensor_units == IPMI_MONITORING_SENSOR_UNITS_RPM)
                sensor_units_str = "RPM";
            else if (sensor_units == IPMI_MONITORING_SENSOR_UNITS_WATTS)
                sensor_units_str = "W";
            else if (sensor_units == IPMI_MONITORING_SENSOR_UNITS_PERCENT)
                sensor_units_str = "%";
            else
                sensor_units_str = "N/A";

            printf (", %s", sensor_units_str);
        }
        else
            printf (", N/A, N/A");

        printf (", %Xh", event_reading_type_code);

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
    return (rv);
}


static int
_ipmimonitoring_sel (struct ipmi_monitoring_ipmi_config *ipmi_config)
{
    ipmi_monitoring_ctx_t ctx = NULL;
    unsigned int sel_flags = 0;
    int i;
    int sel_count;
    int rv = -1;

    if (!(ctx = ipmi_monitoring_ctx_create ()))
    {
        error("ipmi_monitoring_ctx_create()");
        goto cleanup;
    }

    if (sdr_cache_directory)
    {
        if (ipmi_monitoring_ctx_sdr_cache_directory (ctx,
                sdr_cache_directory) < 0)
        {
            error( "ipmi_monitoring_ctx_sdr_cache_directory(): %s",
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
            error( "ipmi_monitoring_ctx_sel_config_file(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else
    {
        if (ipmi_monitoring_ctx_sel_config_file (ctx, NULL) < 0)
        {
            error( "ipmi_monitoring_ctx_sel_config_file(): %s",
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
            error( "ipmi_monitoring_sel_by_record_id(): %s",
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
            error( "ipmi_monitoring_sel_by_sensor_type(): %s",
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
            error( "ipmi_monitoring_sel_by_sensor_type(): %s",
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
            error( "ipmi_monitoring_sel_by_record_id(): %s",
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
            error( "ipmi_monitoring_sel_read_record_id(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((record_type = ipmi_monitoring_sel_read_record_type (ctx)) < 0)
        {
            error( "ipmi_monitoring_sel_read_record_type(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((record_type_class = ipmi_monitoring_sel_read_record_type_class (ctx)) < 0)
        {
            error( "ipmi_monitoring_sel_read_record_type_class(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sel_state = ipmi_monitoring_sel_read_sel_state (ctx)) < 0)
        {
            error( "ipmi_monitoring_sel_read_sel_state(): %s",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        netdata_get_sel(
                  record_id
                , record_type_class
                , sel_state
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
                error( "ipmi_monitoring_sel_read_timestamp(): %s",
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
                error( "ipmi_monitoring_sel_read_sensor_name(): %s",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((sensor_type = ipmi_monitoring_sel_read_sensor_type (ctx)) < 0)
            {
                error( "ipmi_monitoring_sel_read_sensor_type(): %s",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((sensor_number = ipmi_monitoring_sel_read_sensor_number (ctx)) < 0)
            {
                error( "ipmi_monitoring_sel_read_sensor_number(): %s",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_direction = ipmi_monitoring_sel_read_event_direction (ctx)) < 0)
            {
                error( "ipmi_monitoring_sel_read_event_direction(): %s",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_type_code = ipmi_monitoring_sel_read_event_type_code (ctx)) < 0)
            {
                error( "ipmi_monitoring_sel_read_event_type_code(): %s",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if (ipmi_monitoring_sel_read_event_data (ctx,
                    &event_data1,
                    &event_data2,
                    &event_data3) < 0)
            {
                error( "ipmi_monitoring_sel_read_event_data(): %s",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_offset_type = ipmi_monitoring_sel_read_event_offset_type (ctx)) < 0)
            {
                error( "ipmi_monitoring_sel_read_event_offset_type(): %s",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_offset = ipmi_monitoring_sel_read_event_offset (ctx)) < 0)
            {
                error( "ipmi_monitoring_sel_read_event_offset(): %s",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if (!(event_offset_string = ipmi_monitoring_sel_read_event_offset_string (ctx)))
            {
                error( "ipmi_monitoring_sel_read_event_offset_string(): %s",
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
                    error( "ipmi_monitoring_sel_read_manufacturer_id(): %s",
                            ipmi_monitoring_ctx_errormsg (ctx));
                    goto cleanup;
                }

                printf (", Manufacturer ID = %Xh", manufacturer_id);
            }

            if ((oem_data_len = ipmi_monitoring_sel_read_oem_data (ctx, oem_data, 1024)) < 0)
            {
                error( "ipmi_monitoring_sel_read_oem_data(): %s",
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
// MAIN PROGRAM FOR NETDATA PLUGIN

int ipmi_collect_data(struct ipmi_monitoring_ipmi_config *ipmi_config) {
    errno = 0;

    if (_ipmimonitoring_sensors(ipmi_config) < 0) return -1;

    if(netdata_do_sel) {
        if(_ipmimonitoring_sel(ipmi_config) < 0) return -2;
    }

    return 0;
}

int ipmi_detect_speed_secs(struct ipmi_monitoring_ipmi_config *ipmi_config) {
    int i, checks = 10;
    unsigned long long total = 0;

    for(i = 0 ; i < checks ; i++) {
        if(debug) fprintf(stderr, "freeipmi.plugin: checking data collection speed iteration %d of %d\n", i+1, checks);

        // measure the time a data collection needs
        unsigned long long start = now_realtime_usec();
        if(ipmi_collect_data(ipmi_config) < 0)
            fatal("freeipmi.plugin: data collection failed.");

        unsigned long long end = now_realtime_usec();

        if(debug) fprintf(stderr, "freeipmi.plugin: data collection speed was %llu usec\n", end - start);

        // add it to our total
        total += end - start;

        // wait the same time
        // to avoid flooding the IPMI processor with requests
        sleep_usec(end - start);
    }

    // so, we assume it needed 2x the time
    // we find the average in microseconds
    // and we round-up to the closest second

    return (int)(( total * 2 / checks / 1000000 ) + 1);
}

int parse_inband_driver_type (const char *str)
{
    assert (str);

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
    assert (str);

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

int main (int argc, char **argv) {

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

    int i, freq = 0;
    for(i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !freq) {
            int n = str2i(argv[i]);
            if(n > 0 && n < 86400) {
                freq = n;
                continue;
            }
        }
        else if(strcmp("version", argv[i]) == 0 || strcmp("-version", argv[i]) == 0 || strcmp("--version", argv[i]) == 0 || strcmp("-v", argv[i]) == 0 || strcmp("-V", argv[i]) == 0) {
            printf("freeipmi.plugin %s\n", VERSION);
            exit(0);
        }
        else if(strcmp("debug", argv[i]) == 0) {
            debug = 1;
            continue;
        }
        else if(strcmp("sel", argv[i]) == 0) {
            netdata_do_sel = 1;
            continue;
        }
        else if(strcmp("no-sel", argv[i]) == 0) {
            netdata_do_sel = 0;
            continue;
        }
        else if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata freeipmi.plugin %s\n"
                    " Copyright (C) 2016-2017 Costa Tsaousis <costa@tsaousis.gr>\n"
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
                    , VERSION
                    , netdata_update_every
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
            if(debug) fprintf(stderr, "freeipmi.plugin: hostname set to '%s'\n", hostname);
            continue;
        }
        else if(i < argc && strcmp("username", argv[i]) == 0) {
            username = strdupz(argv[++i]);
            char *s = argv[i];
            // mask it be hidden from the process tree
            while(*s) *s++ = 'x';
            if(debug) fprintf(stderr, "freeipmi.plugin: username set to '%s'\n", username);
            continue;
        }
        else if(i < argc && strcmp("password", argv[i]) == 0) {
            password = strdupz(argv[++i]);
            char *s = argv[i];
            // mask it be hidden from the process tree
            while(*s) *s++ = 'x';
            if(debug) fprintf(stderr, "freeipmi.plugin: password set to '%s'\n", password);
            continue;
        }
        else if(strcmp("driver-type", argv[i]) == 0) {
            if (hostname) {
                protocol_version=parse_outofband_driver_type(argv[++i]);
                if(debug) fprintf(stderr, "freeipmi.plugin: outband protocol version set to '%d'\n", protocol_version);
            }
            else {
                driver_type=parse_inband_driver_type(argv[++i]);
                if(debug) fprintf(stderr, "freeipmi.plugin: inband driver type set to '%d'\n", driver_type);
            }
            continue;
        }
        else if(i < argc && strcmp("sdr-cache-dir", argv[i]) == 0) {
            sdr_cache_directory = argv[++i];
            if(debug) fprintf(stderr, "freeipmi.plugin: SDR cache directory set to '%s'\n", sdr_cache_directory);
            continue;
        }
        else if(i < argc && strcmp("sensor-config-file", argv[i]) == 0) {
            sensor_config_file = argv[++i];
            if(debug) fprintf(stderr, "freeipmi.plugin: sensor config file set to '%s'\n", sensor_config_file);
            continue;
        }
        else if(i < argc && strcmp("ignore", argv[i]) == 0) {
            excluded_record_ids_parse(argv[++i]);
            continue;
        }
        else if(i < argc && strcmp("ignore-status", argv[i]) == 0) {
            excluded_status_record_ids_parse(argv[++i]);
            continue;
        }

        error("freeipmi.plugin: ignoring parameter '%s'", argv[i]);
    }

    errno = 0;

    if(freq > netdata_update_every)
        netdata_update_every = freq;

    else if(freq)
        error("update frequency %d seconds is too small for IPMI. Using %d.", freq, netdata_update_every);


    // ------------------------------------------------------------------------
    // initialize IPMI

    struct ipmi_monitoring_ipmi_config ipmi_config;

    if(debug) fprintf(stderr, "freeipmi.plugin: calling _init_ipmi_config()\n");

    _init_ipmi_config(&ipmi_config);

    if(debug) {
        fprintf(stderr, "freeipmi.plugin: calling ipmi_monitoring_init()\n");
        ipmimonitoring_init_flags|=IPMI_MONITORING_FLAGS_DEBUG|IPMI_MONITORING_FLAGS_DEBUG_IPMI_PACKETS;
    }

    if(ipmi_monitoring_init(ipmimonitoring_init_flags, &errnum) < 0)
        fatal("ipmi_monitoring_init: %s", ipmi_monitoring_ctx_strerror(errnum));

    if(debug) fprintf(stderr, "freeipmi.plugin: detecting IPMI minimum update frequency...\n");
    freq = ipmi_detect_speed_secs(&ipmi_config);
    if(debug) fprintf(stderr, "freeipmi.plugin: IPMI minimum update frequency was calculated to %d seconds.\n", freq);

    if(freq > netdata_update_every) {
        info("enforcing minimum data collection frequency, calculated to %d seconds.", freq);
        netdata_update_every = freq;
    }


    // ------------------------------------------------------------------------
    // the main loop

    if(debug) fprintf(stderr, "freeipmi.plugin: starting data collection\n");

    time_t started_t = now_monotonic_sec();

    size_t iteration = 0;
    usec_t step = netdata_update_every * USEC_PER_SEC;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1 ; iteration++) {
        usec_t dt = heartbeat_next(&hb, step);

        if(debug && iteration)
            fprintf(stderr, "freeipmi.plugin: iteration %zu, dt %llu usec, sensors collected %zu, sensors sent to netdata %zu \n"
                    , iteration
                    , dt
                    , netdata_sensors_collected
                    , netdata_sensors_updated
            );

        netdata_mark_as_not_updated();

        if(debug) fprintf(stderr, "freeipmi.plugin: calling ipmi_collect_data()\n");
        if(ipmi_collect_data(&ipmi_config) < 0)
            fatal("data collection failed.");

        if(debug) fprintf(stderr, "freeipmi.plugin: calling send_metrics_to_netdata()\n");
        send_metrics_to_netdata();
        fflush(stdout);

        // restart check (14400 seconds)
        if(now_monotonic_sec() - started_t > 14400) exit(0);
    }
}

#else // !HAVE_FREEIPMI

int main(int argc, char **argv) {
    fatal("freeipmi.plugin is not compiled.");
}

#endif // !HAVE_FREEIPMI
