/*****************************************************************************\
 *  $Id: ipmimonitoring-sensors.c,v 1.51 2016/11/02 23:46:24 chu11 Exp $
 *  $Id: ipmimonitoring-sel.c,v 1.51 2016/11/02 23:46:24 chu11 Exp $
 *****************************************************************************
 *  Copyright (C) 2007-2015 Lawrence Livermore National Security, LLC.
 *  Copyright (C) 2006-2007 The Regents of the University of California.
 *  Produced at Lawrence Livermore National Laboratory (cf, DISCLAIMER).
 *  Written by Albert Chu <chu11@llnl.gov>
 *  UCRL-CODE-222073
 *
 *  This file is part of Ipmimonitoring, an IPMI sensor monitoring
 *  library.  For details, see http://www.llnl.gov/linux/.
 *
 *  Ipmimonitoring is free software; you can redistribute it and/or modify
 *  it under the terms of the GNU General Public License as published by the
 *  Free Software Foundation; either version 3 of the License, or (at your
 *  option) any later version.
 *
 *  Ipmimonitoring is distributed in the hope that it will be useful, but
 *  WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
 *  or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License
 *  for more details.
 *
 *  You should have received a copy of the GNU General Public License along
 *  with Ipmimonitoring.  If not, see <http://www.gnu.org/licenses/>.
\*****************************************************************************/

#include "config.h"

#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <assert.h>
#include <errno.h>
#include <unistd.h>
#include <sys/time.h>

#ifdef HAVE_FREEIPMI

#include <ipmi_monitoring.h>
#include <ipmi_monitoring_bitmasks.h>

/* Communication Configuration - Initialize accordingly */

/* Hostname, NULL for In-band communication, non-null for a hostname */
char *hostname = NULL;

/* In-band Communication Configuration */
int driver_type = IPMI_MONITORING_DRIVER_TYPE_KCS; /* or -1 for default */
int disable_auto_probe = 0;     /* probe for in-band device */
unsigned int driver_address = 0; /* not used if probing */
unsigned int register_spacing = 0; /* not used if probing */
char *driver_device = NULL;     /* not used if probing */

/* Out-of-band Communication Configuration */
int protocol_version = IPMI_MONITORING_PROTOCOL_VERSION_1_5; /* or -1 for default */
char *username = "foousername";
char *password = "foopassword";
unsigned char *k_g = NULL;
unsigned int k_g_len = 0;
int privilege_level = IPMI_MONITORING_PRIVILEGE_LEVEL_USER; /* or -1 for default */
int authentication_type = IPMI_MONITORING_AUTHENTICATION_TYPE_MD5; /* or -1 for default */
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
int ignore_non_interpretable_sensors = 1;
int bridge_sensors = 0;
int interpret_oem_data = 0;
int shared_sensors = 0;
int discrete_reading = 0;
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
    ipmi_config->k_g = k_g;
    ipmi_config->k_g_len = k_g_len;
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

static int netdata_update_every = 5;
static int netdata_priority = 90000;

static size_t netdata_sensors_updated = 0;
static size_t netdata_sensors_collected = 0;
static size_t netdata_sel_events = 0;

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
}

static void send_chart_to_netdata_for_units(int units) {
    struct sensor *sn;

    switch(units) {
        case IPMI_MONITORING_SENSOR_UNITS_CELSIUS:
            printf("CHART ipmi.temperatures_c '' 'System Celcius Temperatures read by IPMI' 'Celcius' 'temperatures' 'ipmi.temperatures_c' 'line' %d %d\n"
                   , netdata_priority
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_FAHRENHEIT:
            printf("CHART ipmi.temperatures_f '' 'System Fahrenheit Temperatures read by IPMI' 'Fahrenheit' 'temperatures' 'ipmi.temperatures_f' 'line' %d %d\n"
                   , netdata_priority
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_VOLTS:
            printf("CHART ipmi.volts '' 'System Voltages read by IPMI' 'Volts' 'voltages' 'ipmi.voltages' 'line' %d %d\n"
                   , netdata_priority
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_AMPS:
            printf("CHART ipmi.amps '' 'System Current read by IPMI' 'Amps' 'current' 'ipmi.amps' 'line' %d %d\n"
                   , netdata_priority
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_RPM:
            printf("CHART ipmi.rpm '' 'System Fans read by IPMI' 'RPM' 'fans' 'ipmi.rpm' 'line' %d %d\n"
                   , netdata_priority
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_WATTS:
            printf("CHART ipmi.watts '' 'System Power read by IPMI' 'Watts' 'power' 'ipmi.watts' 'line' %d %d\n"
                   , netdata_priority
                   , netdata_update_every
            );
            break;

        case IPMI_MONITORING_SENSOR_UNITS_PERCENT:
            printf("CHART ipmi.percent '' 'System Metrics read by IPMI' '%%' 'other' 'ipmi.percent' 'line' %d %d\n"
                   , netdata_priority
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
    static int sel_chart_generated = 0;
    struct sensor *sn;

    if(!sel_chart_generated) {
        printf("CHART ipmi.events '' 'IPMI Events' 'events' 'events' 'ipmi.sel' 'area' %d %d\n"
               , netdata_priority
               , netdata_update_every
        );
        printf("DIMENSION events '' absolute 1 1\n");
    }

    // generate the CHART/DIMENSION lines, if we have to
    for(sn = sensors_root; sn; sn = sn->next)
        if(sn->updated && !sn->exposed && !sn->ignore)
            send_chart_to_netdata_for_units(sn->sensor_units);

    printf("BEGIN ipmi.events\nSET events = %zu\nEND\n", netdata_sel_events);

    // send metrics to netdata
    for(sn = sensors_root; sn; sn = sn->next)
        if(sn->updated && sn->exposed && !sn->sent && !sn->ignore)
            send_metrics_to_netdata_for_units(sn->sensor_units);

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

        sn = calloc(1, sizeof(struct sensor));
        if(!sn) {
            fprintf(stderr, "freeipmi.plugin: cannot allocate %zu bytes of memory.", sizeof(struct sensor));
            exit(1);
        }

        sn->record_id = record_id;
        sn->sensor_number = sensor_number;
        sn->sensor_type = sensor_type;
        sn->sensor_state = sensor_state;
        sn->sensor_units = sensor_units;
        sn->sensor_reading_type = sensor_reading_type;
        sn->sensor_name = strdup(sensor_name);
        if(!sn->sensor_name) {
            fprintf(stderr, "freeipmi.plugin: cannot allocate %zu bytes of memory.", strlen(sensor_name));
            exit(1);
        }

        sn->next = sensors_root;
        sensors_root = sn;
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
            sn->ignore = 1;
            break;
    }

/*    switch(sensor_state) {
        case IPMI_MONITORING_STATE_NOMINAL:
        case IPMI_MONITORING_STATE_WARNING:
        case IPMI_MONITORING_STATE_CRITICAL:
        default:
            break;
    }
*/
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


static unsigned long long now_realtime_usec() {
    struct timeval tv;
    gettimeofday(&tv, NULL);
    return tv.tv_sec * 1000000ULL + tv.tv_usec;
}

// END NETDATA CODE
// ----------------------------------------------------------------------------


/* This is an example of how to use the libipmimonitoring library to
 * read and monitor sensors.
 *
 * At the top of this file, you'll find a number of variables for
 * configuration of IPMI communication and what sensors you are
 * interested in monitoring.  Those variables are used in the
 * libipmimonitoring calls below.
 *
 * Hopefully this example will be sufficient to help anyone program
 * IPMI monitoring software for their environment.
 *
 * To compile, linking against the library should be sufficient for
 * most environments.  e.g.
 *
 * gcc -o freeipmi.plugin freeipmi_plugin.c -lipmimonitoring
 */

static int
_ipmimonitoring_sensors (struct ipmi_monitoring_ipmi_config *ipmi_config)
{
    ipmi_monitoring_ctx_t ctx = NULL;
    unsigned int sensor_reading_flags = 0;
    int i;
    int sensor_count;
    int rv = -1;

    if (!(ctx = ipmi_monitoring_ctx_create ()))
    {
        perror ("ipmi_monitoring_ctx_create:");
        goto cleanup;
    }

    if (sdr_cache_directory)
    {
        if (ipmi_monitoring_ctx_sdr_cache_directory (ctx,
                sdr_cache_directory) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_ctx_sdr_cache_directory: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_ctx_sensor_config_file: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else
    {
        if (ipmi_monitoring_ctx_sensor_config_file (ctx, NULL) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_ctx_sensor_config_file: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sensor_readings_by_record_id: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sensor_readings_by_record_id: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sensor_readings_by_sensor_type: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_record_id: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sensor_number = ipmi_monitoring_sensor_read_sensor_number (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_number: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sensor_type = ipmi_monitoring_sensor_read_sensor_type (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_type: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if (!(sensor_name = ipmi_monitoring_sensor_read_sensor_name (ctx)))
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_name: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sensor_state = ipmi_monitoring_sensor_read_sensor_state (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_state: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sensor_units = ipmi_monitoring_sensor_read_sensor_units (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_units: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

#ifdef NETDATA_COMMENTED
        if ((sensor_bitmask_type = ipmi_monitoring_sensor_read_sensor_bitmask_type (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_bitmask_type: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
        if ((sensor_bitmask = ipmi_monitoring_sensor_read_sensor_bitmask (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_bitmask: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if (!(sensor_bitmask_strings = ipmi_monitoring_sensor_read_sensor_bitmask_strings (ctx)))
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_bitmask_strings: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
#endif // NETDATA_COMMENTED

        if ((sensor_reading_type = ipmi_monitoring_sensor_read_sensor_reading_type (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_sensor_reading_type: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        sensor_reading = ipmi_monitoring_sensor_read_sensor_reading (ctx);

#ifdef NETDATA_COMMENTED
        if ((event_reading_type_code = ipmi_monitoring_sensor_read_event_reading_type_code (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sensor_read_event_reading_type_code: %s\n",
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

        printf ("%u, %s, %u, %s",
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

        if (sensor_bitmask_type != IPMI_MONITORING_SENSOR_BITMASK_TYPE_UNKNOWN)
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


/* This is an example of how to use the libipmimonitoring library to
 * read and monitor the SEL.
 *
 * At the top of this file, you'll find a number of variables for
 * configuration of IPMI communication and what SEL records you are
 * interested in monitoring.  Those variables are used in the
 * libipmimonitoring calls below.
 *
 * Hopefully this example will be sufficient to help anyone program
 * IPMI monitoring software for their environment.
 *
 * To compile, linking against the library should be sufficient for
 * most environments.  e.g.
 *
 * gcc -o ipmimonitoring-sel ipmimonitoring-sel.c -lipmimonitoring
 */

/* Communication Configuration - Initialize accordingly */

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
        perror ("ipmi_monitoring_ctx_create:");
        goto cleanup;
    }

    if (sdr_cache_directory)
    {
        if (ipmi_monitoring_ctx_sdr_cache_directory (ctx,
                sdr_cache_directory) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_ctx_sdr_cache_directory: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_ctx_sel_config_file: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }
    }
    else
    {
        if (ipmi_monitoring_ctx_sel_config_file (ctx, NULL) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_ctx_sel_config_file: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sel_by_record_id: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sel_by_sensor_type: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sel_by_sensor_type: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sel_by_record_id: %s\n",
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
            fprintf (stderr,
                    "ipmi_monitoring_sel_read_record_id: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((record_type = ipmi_monitoring_sel_read_record_type (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sel_read_record_type: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((record_type_class = ipmi_monitoring_sel_read_record_type_class (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sel_read_record_type_class: %s\n",
                    ipmi_monitoring_ctx_errormsg (ctx));
            goto cleanup;
        }

        if ((sel_state = ipmi_monitoring_sel_read_sel_state (ctx)) < 0)
        {
            fprintf (stderr,
                    "ipmi_monitoring_sel_read_sel_state: %s\n",
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

        printf ("%u, %u, %s",
                record_id,
                record_type,
                sel_state_str);

        if (record_type_class == IPMI_MONITORING_SEL_RECORD_TYPE_CLASS_SYSTEM_EVENT_RECORD
            || record_type_class == IPMI_MONITORING_SEL_RECORD_TYPE_CLASS_TIMESTAMPED_OEM_RECORD)
        {

            if (ipmi_monitoring_sel_read_timestamp (ctx, &timestamp) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_timestamp: %s\n",
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
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_sensor_name: %s\n",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((sensor_type = ipmi_monitoring_sel_read_sensor_type (ctx)) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_sensor_type: %s\n",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((sensor_number = ipmi_monitoring_sel_read_sensor_number (ctx)) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_sensor_number: %s\n",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_direction = ipmi_monitoring_sel_read_event_direction (ctx)) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_event_direction: %s\n",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_type_code = ipmi_monitoring_sel_read_event_type_code (ctx)) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_event_type_code: %s\n",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if (ipmi_monitoring_sel_read_event_data (ctx,
                    &event_data1,
                    &event_data2,
                    &event_data3) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_event_data: %s\n",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_offset_type = ipmi_monitoring_sel_read_event_offset_type (ctx)) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_event_offset_type: %s\n",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if ((event_offset = ipmi_monitoring_sel_read_event_offset (ctx)) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_event_offset: %s\n",
                        ipmi_monitoring_ctx_errormsg (ctx));
                goto cleanup;
            }

            if (!(event_offset_string = ipmi_monitoring_sel_read_event_offset_string (ctx)))
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_event_offset_string: %s\n",
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

            printf (", %s, %s, %u, %s, %Xh, %Xh-%Xh-%Xh",
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
                    fprintf (stderr,
                            "ipmi_monitoring_sel_read_manufacturer_id: %s\n",
                            ipmi_monitoring_ctx_errormsg (ctx));
                    goto cleanup;
                }

                printf (", Manufacturer ID = %Xh", manufacturer_id);
            }

            if ((oem_data_len = ipmi_monitoring_sel_read_oem_data (ctx, oem_data, 1024)) < 0)
            {
                fprintf (stderr,
                        "ipmi_monitoring_sel_read_oem_data: %s\n",
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

    if (_ipmimonitoring_sensors(ipmi_config) < 0) return -1;
    if (_ipmimonitoring_sel    (ipmi_config) < 0) return -2;

    return 0;
}

int ipmi_detect_speed_secs(struct ipmi_monitoring_ipmi_config *ipmi_config) {
    int i, checks = 10;
    unsigned long long total = 0;

    for(i = 0 ; i < checks ; i++) {
        if(debug) fprintf(stderr, "freeipmi.plugin: checking data collection speed iteration %d of %d\n", i+1, checks);

        // measure the time a data collection needs
        unsigned long long start = now_realtime_usec();
        if(ipmi_collect_data(ipmi_config) < 0) {
            fprintf(stderr, "freeipmi.plugin: data collection failed.\n");
            exit(1);
        }
        unsigned long long end = now_realtime_usec();

        if(debug) fprintf(stderr, "freeipmi.plugin: data collection speed was %llu usec\n", end - start);

        // add it to our total
        total += end - start;

        // wait the same time
        // to avoid flooding the IPMI processor with requests
        usleep(end - start);
    }

    // so, we assume it needed 3x the time
    // we find the average in microseconds
    // and we round-up to the closest second

    return (( total * 3 / checks / 1000000 ) + 1);
}

int main (int argc, char **argv) {
    struct ipmi_monitoring_ipmi_config ipmi_config;

    // parse command line parameters

    int i, freq = 0;
    for(i = 1; i < argc ; i++) {
        if(!freq) {
            int n = atoi(argv[i]);
            if(n > 0) {
                freq = n;
                continue;
            }
        }

        if(strcmp("debug", argv[i]) == 0) {
            debug = 1;
            continue;
        }

        fprintf(stderr, "freeipmi.plugin: ignoring parameter '%s'\n", argv[i]);
    }

    if(freq > 0 && freq < netdata_update_every)
        netdata_update_every = freq;

    else if(freq)
        fprintf(stderr, "freeipmi.plugin: update frequency %d seconds is too small for IPMI. Using %d", freq, netdata_update_every);


    // initialize IPMI

    if(debug) fprintf(stderr, "freeipmi.plugin: calling _init_ipmi_config()\n");

    _init_ipmi_config(&ipmi_config);

    if(debug) fprintf(stderr, "freeipmi.plugin: calling ipmi_monitoring_init()\n");

    if(ipmi_monitoring_init(ipmimonitoring_init_flags, &errnum) < 0) {
        fprintf(stderr, "ipmi_monitoring_init: %s\n", ipmi_monitoring_ctx_strerror(errnum));
        exit(1);
    }

    if(debug) fprintf(stderr, "freeipmi.plugin: detecting IPMI minimum update frequency...\n");
    freq = ipmi_detect_speed_secs(&ipmi_config);
    if(debug) fprintf(stderr, "freeipmi.plugin: IPMI minimum update frequency was calculated to %d seconds.\n", freq);

    if(netdata_update_every < freq) {
        fprintf(stderr, "freeipmi.plugin: enforcing minimum data collection frequency, calculated to %d seconds.\n", freq);
        netdata_update_every = freq;
    }

    // the main loop
    if(debug) fprintf(stderr, "freeipmi.plugin: starting data collection\n");

    size_t iteration = 0;
    unsigned long long step = netdata_update_every * 1000000ULL;
    unsigned long long now = now_realtime_usec();
    unsigned long long next = now - (now % step) + step;
    while(1) {
        unsigned long long last = now;
        now = now_realtime_usec();
        if(debug && iteration)
            fprintf(stderr, "freeipmi.plugin: iteration %zu, dt %llu usec, sensors collected %zu, sensors sent to netdata %zu \n"
                    , iteration
                    , now - last
                    , netdata_sensors_collected
                    , netdata_sensors_updated
            );

        while(now < next) {
            if(debug) fprintf(stderr, "freeipmi.plugin: sleeping for %llu usec\n", next - now);
            usleep(next - now);
            now = now_realtime_usec();
        }
        next = now - (now % step) + step;

        netdata_mark_as_not_updated();

        if(debug) fprintf(stderr, "freeipmi.plugin: calling ipmi_collect_data()\n");
        if(ipmi_collect_data(&ipmi_config) < 0) {
            fprintf(stderr, "freeipmi.plugin: data collection failed.\n");
            exit(1);
        }

        if(debug) fprintf(stderr, "freeipmi.plugin: calling send_metrics_to_netdata()\n");
        send_metrics_to_netdata();
        fflush(stdout);

        iteration++;
    }
    exit(0);
}

#else // !HAVE_FREEIPMI

int main(int argc, char **argv) {
    fprintf(stderr, "freeipmi.plugin: not compiled.");
    exit(1);
}

#endif
