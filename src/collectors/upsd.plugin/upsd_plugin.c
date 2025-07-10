#include <assert.h>
#include <errno.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include <upsclient.h>

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#define PLUGIN_UPSD_NAME "upsd.plugin"

#define NETDATA_PLUGIN_EXIT_AND_RESTART 0
#define NETDATA_PLUGIN_EXIT_AND_DISABLE 1

#define NETDATA_CLABEL_SOURCE_AUTO   1
#define NETDATA_CLABEL_SOURCE_MANUAL 2
#define NETDATA_CLABEL_SOURCE_K8     4
#define NETDATA_CLABEL_SOURCE_AGENT  8

#define NETDATA_CHART_PRIO_UPSD_UPS_LOAD      70000
#define NETDATA_CHART_PRIO_UPSD_UPS_LOADUSAGE 70001
#define NETDATA_CHART_PRIO_UPSD_UPS_STATUS    70002
#define NETDATA_CHART_PRIO_UPSD_UPS_TEMP      70003

#define NETDATA_CHART_PRIO_UPSD_BATT_CHARGE      70004
#define NETDATA_CHART_PRIO_UPSD_BATT_RUNTIME     70005
#define NETDATA_CHART_PRIO_UPSD_BATT_VOLTAGE     70006
#define NETDATA_CHART_PRIO_UPSD_BATT_VOLTAGE_NOM 70007

#define NETDATA_CHART_PRIO_UPSD_INPT_VOLTAGE       70008
#define NETDATA_CHART_PRIO_UPSD_INPT_VOLTAGE_NOM   70009
#define NETDATA_CHART_PRIO_UPSD_INPT_CURRENT       70010
#define NETDATA_CHART_PRIO_UPSD_INPT_CURRENT_NOM   70011
#define NETDATA_CHART_PRIO_UPSD_INPT_FREQUENCY     70012
#define NETDATA_CHART_PRIO_UPSD_INPT_FREQUENCY_NOM 70013

#define NETDATA_CHART_PRIO_UPSD_OUPT_VOLTAGE       70014
#define NETDATA_CHART_PRIO_UPSD_OUPT_VOLTAGE_NOM   70015
#define NETDATA_CHART_PRIO_UPSD_OUPT_CURRENT       70016
#define NETDATA_CHART_PRIO_UPSD_OUPT_CURRENT_NOM   70017
#define NETDATA_CHART_PRIO_UPSD_OUPT_FREQUENCY     70018
#define NETDATA_CHART_PRIO_UPSD_OUPT_FREQUENCY_NOM 70019

#define NETDATA_PLUGIN_PRECISION 100

#define BUFLEN 64
#define LENGTHOF(arr) (sizeof(arr)/sizeof(arr[0]))

static bool debug = false;
static unsigned long netdata_update_every = 1;
UPSCONN_t ups1, ups2;

// Hash table mapping UPS name to another hashtable, which maps NUT variable string to
// struct chart pointer.
DICTIONARY *nd_ups_vars;

// Hash table mapping UPS name to a boolean value indicating whether or not the UPS was
// observed in the most recent 'LIST UPS' query.
DICTIONARY *nd_ups_seen;

// Hash table mapping UPS name to a 'cleaned' (normalized) version of the mapping name
// which is suitable for use in NetData.
DICTIONARY *nd_ups_name;

// https://networkupstools.org/docs/developer-guide.chunked/new-drivers.html#_status_data
struct nut_ups_status {
    unsigned int OL      : 1; // On line
    unsigned int OB      : 1; // On battery
    unsigned int LB      : 1; // Low battery
    unsigned int HB      : 1; // High battery
    unsigned int RB      : 1; // The battery needs to be replaced
    unsigned int CHRG    : 1; // The battery is charging
    unsigned int DISCHRG : 1; // The battery is discharging (inverter is providing load power)
    unsigned int BYPASS  : 1; // UPS bypass circuit is active -- no battery protection is available
    unsigned int CAL     : 1; // UPS is currently performing runtime calibration (on battery)
    unsigned int OFF     : 1; // UPS is offline and is not supplying power to the load
    unsigned int OVER    : 1; // UPS is overloaded
    unsigned int TRIM    : 1; // UPS is trimming incoming voltage (called "buck" in some hardware)
    unsigned int BOOST   : 1; // UPS is boosting incoming voltage
    unsigned int FSD     : 1; // Forced Shutdown
    unsigned int OTHER   : 1;
};

// https://learn.netdata.cloud/docs/developer-and-contributor-corner/external-plugins/#chart
struct nd_chart {
    const char *nut_variable;
    const char *chart_id;
    const char *chart_name;
    const char *chart_title;
    const char *chart_units;
    const char *chart_family;
    const char *chart_context;
    const char *chart_type;
    unsigned int chart_priority;
    const char *chart_dimension;
};

struct nd_chart nd_charts[] = {
    {
        .nut_variable = "ups.load",
        .chart_id = "load_percentage",
        .chart_title = "UPS load",
        .chart_units = "percentage",
        .chart_family = "ups",
        .chart_context = "upsd.ups_load",
        .chart_type = "area",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_UPS_LOAD,
        .chart_dimension = "load",
    },
    {
        .nut_variable = "ups.realpower",
        .chart_id = "load_usage",
        .chart_title = "UPS load usage (power output)",
        .chart_units = "Watts",
        .chart_family = "ups",
        .chart_context = "upsd.ups_load_usage",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_UPS_LOADUSAGE,
        .chart_dimension = "load_usage",
    },
    {
        .nut_variable = "ups.temperature",
        .chart_id = "temperature",
        .chart_title = "UPS temperature",
        .chart_units = "Celsius",
        .chart_family = "ups",
        .chart_context = "upsd.ups_temperature",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_UPS_TEMP,
        .chart_dimension = "temperature",
    },
    {
        .nut_variable = "battery.charge",
        .chart_id = "battery_charge_percentage",
        .chart_title = "UPS Battery charge",
        .chart_units = "percentage",
        .chart_family = "battery",
        .chart_context = "upsd.ups_battery_charge",
        .chart_type = "area",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_BATT_CHARGE,
        .chart_dimension = "charge",
    },
    {
        .nut_variable = "battery.runtime",
        .chart_id = "battery_estimated_runtime",
        .chart_title = "UPS Battery estimated runtime",
        .chart_units = "seconds",
        .chart_family = "battery",
        .chart_context = "upsd.ups_battery_estimated_runtime",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_BATT_RUNTIME,
        .chart_dimension = "runtime",
    },
    {
        .nut_variable = "battery.voltage",
        .chart_id = "battery_voltage",
        .chart_title = "UPS Battery voltage",
        .chart_units = "Volts",
        .chart_family = "battery",
        .chart_context = "upsd.ups_battery_voltage",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_BATT_VOLTAGE,
        .chart_dimension = "voltage",
    },
    {
        .nut_variable = "battery.voltage.nominal",
        .chart_id = "battery_voltage_nominal",
        .chart_title = "UPS Battery voltage nominal",
        .chart_units = "Volts",
        .chart_family = "battery",
        .chart_context = "upsd.ups_battery_voltage_nominal",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_BATT_VOLTAGE_NOM,
        .chart_dimension = "nominal_voltage",
    },
    {
        .nut_variable = "input.voltage",
        .chart_id = "input_voltage",
        .chart_title = "UPS Input voltage",
        .chart_units = "Volts",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_voltage",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_INPT_VOLTAGE,
        .chart_dimension = "voltage",
    },
    {
        .nut_variable = "input.voltage.nominal",
        .chart_id = "input_voltage_nominal",
        .chart_title = "UPS Input voltage nominal",
        .chart_units = "Volts",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_voltage_nominal",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_INPT_VOLTAGE_NOM,
        .chart_dimension = "nominal_voltage",
    },
    {
        .nut_variable = "input.current",
        .chart_id = "input_current",
        .chart_title = "UPS Input current",
        .chart_units = "Ampere",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_current",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_INPT_CURRENT,
        .chart_dimension = "current",
    },
    {
        .nut_variable = "input.current.nominal",
        .chart_id = "input_current_nominal",
        .chart_title = "UPS Input current nominal",
        .chart_units = "Ampere",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_current_nominal",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_INPT_CURRENT_NOM,
        .chart_dimension = "nominal_current",
    },
    {
        .nut_variable = "input.frequency",
        .chart_id = "input_frequency",
        .chart_title = "UPS Input frequency",
        .chart_units = "Hz",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_frequency",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_INPT_FREQUENCY,
        .chart_dimension = "frequency",
    },
    {
        .nut_variable = "input.frequency.nominal",
        .chart_id = "input_frequency_nominal",
        .chart_title = "UPS Input frequency nominal",
        .chart_units = "Hz",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_frequency_nominal",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_INPT_FREQUENCY_NOM,
        .chart_dimension = "nominal_frequency",
    },
    {
        .nut_variable = "output.voltage",
        .chart_id = "output_voltage",
        .chart_title = "UPS Output voltage",
        .chart_units = "Volts",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_voltage",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_OUPT_VOLTAGE,
        .chart_dimension = "voltage",
    },
    {
        .nut_variable = "output.voltage.nominal",
        .chart_id = "output_voltage_nominal",
        .chart_title = "UPS Output voltage nominal",
        .chart_units = "Volts",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_voltage_nominal",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_OUPT_VOLTAGE_NOM,
        .chart_dimension = "nominal_voltage",
    },
    {
        .nut_variable = "output.current",
        .chart_id = "output_current",
        .chart_title = "UPS Output current",
        .chart_units = "Ampere",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_current",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_OUPT_CURRENT,
        .chart_dimension = "current",
    },
    {
        .nut_variable = "output.current.nominal",
        .chart_id = "output_current_nominal",
        .chart_title = "UPS Output current nominal",
        .chart_units = "Ampere",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_current_nominal",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_OUPT_CURRENT_NOM,
        .chart_dimension = "nominal_current",
    },
    {
        .nut_variable = "output.frequency",
        .chart_id = "output_frequency",
        .chart_title = "UPS Output frequency",
        .chart_units = "Hz",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_frequency",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_OUPT_FREQUENCY,
        .chart_dimension = "frequency",
    },
    {
        .nut_variable = "output.frequency.nominal",
        .chart_id = "output_frequency_nominal",
        .chart_title = "UPS Output frequency nominal",
        .chart_units = "Hz",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_frequency_nominal",
        .chart_type = "line",
        .chart_priority = NETDATA_CHART_PRIO_UPSD_OUPT_FREQUENCY_NOM,
        .chart_dimension = "nominal_frequency",
    },
    { 0 },
};

static void print_version()
{
    fputs("netdata " PLUGIN_UPSD_NAME " " NETDATA_VERSION "\n"
          "\n"
          "Copyright 2025 Netdata Inc.\n"
          "Original Author: Mario Campos <mario.andres.campos@gmail.com>\n"
          "Released under GNU General Public License v3+.\n"
          "\n"
          "This program is a data collector plugin for netdata.\n",
          stderr
    );
}

static void print_help() {
    fputs("usage: " PLUGIN_UPSD_NAME " [-d] [COLLECTION_FREQUENCY]\n"
          "       " PLUGIN_UPSD_NAME " -v\n"
          "       " PLUGIN_UPSD_NAME " -h\n"
          "\n"
          "options:\n"
          "  COLLECTION_FREQUENCY    data collection frequency in seconds (default: 1)\n"
          "  -d                      enable verbose output (default: disabled)\n"
          "  -v                      print version and exit\n"
          "  -h                      print this message and exit\n",
          stderr
    );
}

// Netdata will call the plugin with just one command line parameter: the number of
// seconds the user requested this plugin to update its data (by default is also 1).
static void parse_command_line(int argc, char *argv[]) {
    int opt;
    char *endptr;

    while ((opt = getopt(argc, argv, "hvd")) != -1) {
        switch (opt) {
        case 'h':
            print_help();
            exit(EXIT_SUCCESS);
        case 'v':
            print_version();
            exit(EXIT_SUCCESS);
        case 'd':
            debug = true;
            break;
        default:
            print_help();
            exit(EXIT_FAILURE);
        }
    }

    if (optind == argc)
        netdata_update_every = 1;
    else {
        if (!isdigit(*argv[optind])) {
            print_help();
            exit(EXIT_FAILURE);
        }
        netdata_update_every = str2i(argv[optind]);
        if (netdata_update_every <= 0 || netdata_update_every >= 86400) {
            netdata_log_error("COLLECTION_FREQUENCY argument must be between [1,86400)\n");
            exit(EXIT_FAILURE);
        }
    }
}

static char *clean_name(char *name) {
    assert(name);
    for (char *c = name; *c; c++)
        *c = (*c  == ' ' || *c == '.') ? '_' : *c;
    return name;
}

static void delete_unseen_ups(void) {
    bool *seen;
    dfe_start_read(nd_ups_seen, seen) {
        if (*seen) {
            *seen = false;
        } else {
            DICTIONARY *ups_vars = dictionary_get(nd_ups_vars, seen_dfe.name);
            dictionary_destroy(ups_vars);
            dictionary_del(nd_ups_vars, seen_dfe.name);
            dictionary_del(nd_ups_seen, seen_dfe.name);
            dictionary_del(nd_ups_name, seen_dfe.name);
        }
    }
    dfe_done(seen);
}

static const char *nut_get_var(UPSCONN_t *conn, const char *ups_name, const char *var_name) {
    assert(conn);
    assert(ups_name);
    assert(var_name);

    size_t numa;
    char **answer[1];
    const char *query[] = { "VAR", ups_name, var_name };

    if (-1 == upscli_get(conn, LENGTHOF(query), query, &numa, (char***)answer)) {
        assert(upscli_upserror(conn) == UPSCLI_ERR_VARNOTSUPP);
        return NULL;
    }

    // The output of upscli_get() will be something like:
    //   { { [0] = "VAR", [1] = <UPS name>, [2] = <variable name>, [3] = <variable value> } }
    return answer[0][3];
}

static inline void send_BEGIN(const char *type, const char *name, usec_t usec) {
    printf("BEGIN upsd_%s.%s %" PRIu64 "\n", type, name, usec);
}

static inline void send_SET(const char *name, unsigned int value) {
    printf("SET %s = %u\n", name, value);
}

static inline void send_END(void) {
    puts("END");
}

// This function parses the 'ups.status' variable and emits the Netdata metrics
// for each status, printing 1 for each set status and 0 otherwise.
static void send_metric_ups_status(const char *ups_name, const char *clean_ups_name, usec_t dt) {
    assert(ups_name);
    assert(clean_ups_name);

    struct nut_ups_status status = { 0 };
    const char *ups_status_string = nut_get_var(&ups2, ups_name, "ups.status");

    for (const char *c = ups_status_string; c && *c; c++) {
        switch (*c) {
        case ' ':
            continue;
        case 'L':
            c++;
            status.LB = 1;
            break;
        case 'H':
            c++;
            status.HB = 1;
            break;
        case 'R':
            c++;
            status.RB = 1;
            break;
        case 'D':
            c += 6;
            status.DISCHRG = 1;
            break;
        case 'T':
            c += 3;
            status.TRIM = 1;
            break;
        case 'F':
            c += 2;
            status.FSD = 1;
            break;
        case 'B':
            switch (*++c) {
            case 'O':
                c += 3;
                status.BOOST = 1;
                break;
            case 'Y':
                c += 4;
                status.BYPASS = 1;
                break;
            default:
                status.OTHER = 1;
                c = strchr(c, ' ');
                break;
            }
            break;
        case 'C':
            switch (*++c) {
            case 'H':
                c += 2;
                status.CHRG = 1;
                break;
            case 'A':
                c++;
                status.CAL = 1;
                break;
            default:
                status.OTHER = 1;
                c = strchr(c, ' ');
                break;
            }
            break;
        case 'O':
            switch (*++c) {
            case 'B':
                status.OB = 1;
                break;
            case 'F':
                status.OFF = 1;
                break;
            case 'L':
                status.OL = 1;
                break;
            case 'V':
                c += 2;
                status.OVER = 1;
                break;
            default:
                status.OTHER = 1;
                c = strchr(c, ' ');
                break;
            }
            break;
        default:
            status.OTHER = 1;
            c = strchr(c, ' ');
            break;
        }
    }

    send_BEGIN(clean_ups_name, "status", dt);
    send_SET("on_line", status.OL);
    send_SET("on_battery", status.OB);
    send_SET("low_battery", status.LB);
    send_SET("high_battery", status.HB);
    send_SET("replace_battery", status.RB);
    send_SET("charging", status.CHRG);
    send_SET("discharging", status.DISCHRG);
    send_SET("bypass", status.BYPASS);
    send_SET("calibration", status.CAL);
    send_SET("offline", status.OFF);
    send_SET("overloaded", status.OVER);
    send_SET("trim_input_voltage", status.TRIM);
    send_SET("boost_input_voltage", status.BOOST);
    send_SET("forced_shutdown", status.FSD);
    send_SET("other", status.OTHER);
    send_END();
}

static void send_metric_ups_realpower(const char *ups_name, const char *clean_ups_name, usec_t dt) {
    assert(ups_name);
    assert(clean_ups_name);

    NETDATA_DOUBLE realpower;
    const char *value = nut_get_var(&ups2, ups_name, "ups.realpower");

    if (value) {
        realpower = str2ndd(value, NULL) * NETDATA_PLUGIN_PRECISION;
    } else {
        value = nut_get_var(&ups2, ups_name, "ups.load");
        if (!value)
            return;
        realpower = str2ndd(value, NULL) / 100;
        value = nut_get_var(&ups2, ups_name, "ups.realpower.nominal");
        if (!value)
            return;
        realpower *= str2ndd(value, NULL) * NETDATA_PLUGIN_PRECISION;
    }

    send_BEGIN(clean_ups_name, "load_usage", dt);
    send_SET("load_usage", realpower);
    send_END();
}

static void register_ups(char *ups_name) {
    const char *nut_value;
    const char *clean_ups_name = clean_name(dictionary_set(nd_ups_name, ups_name, ups_name, strlen(ups_name)+1));

    // CHART type.id name title units [family [context [charttype [priority [update_every [options [plugin [module]]]]]]]]
    printf("CHART 'upsd_%s.status' '' 'UPS status' 'status' 'ups' 'upsd.ups_status' 'line' %u %u\n",
           clean_ups_name, NETDATA_CHART_PRIO_UPSD_UPS_STATUS, netdata_update_every);

    if ((nut_value = nut_get_var(&ups2, ups_name, "battery.type")))
        printf("CLABEL battery_type '%s' %u\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);
    if ((nut_value = nut_get_var(&ups2, ups_name, "device.model")))
        printf("CLABEL device_model '%s' %u\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);
    if ((nut_value = nut_get_var(&ups2, ups_name, "device.serial")))
        printf("CLABEL device_serial '%s' %u\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);
    if ((nut_value = nut_get_var(&ups2, ups_name, "device.mfr")))
        printf("CLABEL device_manufacturer '%s' %u\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);
    if ((nut_value = nut_get_var(&ups2, ups_name, "device.type")))
        printf("CLABEL device_type '%s' %u\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);

    // CLABEL name value source
    // CLABEL_COMMIT
    printf("CLABEL ups_name '%s' %u\n"
           "CLABEL_COMMIT\n",
           ups_name, NETDATA_CLABEL_SOURCE_AUTO);

    // DIMENSION id [name [algorithm [multiplier [divisor [options]]]]]
    printf("DIMENSION on_line '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION on_battery '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION low_battery '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION high_battery '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION replace_battery '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION charging '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION discharging '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION bypass '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION calibration '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION offline '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION overloaded '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION trim_input_voltage '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION boost_input_voltage '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION forced_shutdown '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);
    printf("DIMENSION other '' '' '' %u\n", NETDATA_PLUGIN_PRECISION);

    // Hash table mapping NUT variable (e.g. 'ups.status') to pointer to respective `struct nd_chart`.
    DICTIONARY *ups_vars = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_FIXED_SIZE|DICT_OPTION_NAME_LINK_DONT_CLONE|DICT_OPTION_VALUE_LINK_DONT_CLONE);

    for (struct nd_chart *chart = nd_charts; chart->nut_variable; chart++) {
        nut_value = nut_get_var(&ups2, ups_name, chart->nut_variable);
        if (!nut_value) {
            if (!streq(chart->nut_variable, "ups.realpower"))
                continue;
            // If the UPS does not support the 'ups.realpower' variable, then
            // we can still calculate the load_usage if the 'ups.load' and
            // 'ups.realpower.nominal' variables are available.
            if (!nut_get_var(&ups2, ups_name, "ups.load") || !nut_get_var(&ups2, ups_name, "ups.realpower.nominal"))
                continue;
        }

        // CHART type.id name title units [family [context [charttype [priority [update_every [options [plugin [module]]]]]]]]
        printf("CHART 'upsd_%s.%s' '' '%s' '%s' '%s' '%s' '%s' '%u' '%u' '' '" PLUGIN_UPSD_NAME "'\n",
               clean_ups_name, chart->chart_id, // type.id
               chart->chart_title,    // title
               chart->chart_units,    // units
               chart->chart_family,   // family
               chart->chart_context,  // context
               chart->chart_type,     // charttype
               chart->chart_priority, // priority
               netdata_update_every); // update_every

        if ((nut_value = nut_get_var(&ups2, ups_name, "battery.type")))
            printf("CLABEL 'battery_type' '%s' '%u'\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);
        if ((nut_value = nut_get_var(&ups2, ups_name, "device.model")))
            printf("CLABEL 'device_model' '%s' '%u'\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);
        if ((nut_value = nut_get_var(&ups2, ups_name, "device.serial")))
            printf("CLABEL 'device_serial' '%s' '%u'\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);
        if ((nut_value = nut_get_var(&ups2, ups_name, "device.mfr")))
            printf("CLABEL 'device_manufacturer' '%s' '%u'\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);
        if ((nut_value = nut_get_var(&ups2, ups_name, "device.type")))
            printf("CLABEL 'device_type' '%s' '%u'\n", nut_value, NETDATA_CLABEL_SOURCE_AUTO);

        // CLABEL name value source
        // CLABEL_COMMIT
        printf("CLABEL 'ups_name' '%s' %u\n"
               "CLABEL_COMMIT\n",
               ups_name, NETDATA_CLABEL_SOURCE_AUTO);

        // DIMENSION id [name [algorithm [multiplier [divisor [options]]]]]
        printf("DIMENSION '%s' '' '' '' %u\n", chart->chart_dimension, NETDATA_PLUGIN_PRECISION);

        dictionary_set(ups_vars, chart->nut_variable, chart, 0);
    }
    dictionary_set(nd_ups_vars, ups_name, ups_vars, 0);
}

int main(int argc, char *argv[]) {
    int rc;
    size_t numa;
    char **answer[1];
    const char *query[] = { "UPS" };
    struct nd_chart *chart;

    parse_command_line(argc, argv);

    nd_log_initialize_for_external_plugins(PLUGIN_UPSD_NAME);
    netdata_threads_init_for_external_plugins(0);

    nd_ups_vars = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_FIXED_SIZE|DICT_OPTION_VALUE_LINK_DONT_CLONE);
    nd_ups_seen = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_FIXED_SIZE);
    nd_ups_name = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    // If we fail to initialize libupsclient or connect to a local
    // UPS, then there's nothing more to be done; Netdata should disable
    // this plugin, since it cannot offer any metrics.
    if (-1 == upscli_init(0, NULL, NULL, NULL)) {
        netdata_log_error("failed to initialize libupsclient");
        return NETDATA_PLUGIN_EXIT_AND_DISABLE;
    }

    if ((-1 == upscli_connect(&ups1, "127.0.0.1", 3493, 0)) ||
        (-1 == upscli_connect(&ups2, "127.0.0.1", 3493, 0))) {
        upscli_cleanup();
        netdata_log_error("failed to connect to upsd at 127.0.0.1:3493");
        return NETDATA_PLUGIN_EXIT_AND_DISABLE;
    }

    // Set stdout to block-buffered, to make printf() faster.
    setvbuf(stdout, NULL, _IOFBF, BUFSIZ);

    rc = upscli_list_start(&ups1, LENGTHOF(query), query);
    if (unlikely(-1 == rc)) {
        netdata_log_error("failed to list UPSes from upsd: %s", upscli_upserror(&ups1));
        return NETDATA_PLUGIN_EXIT_AND_DISABLE;
    }

    for (;;) {
        // The output of upscli_list_next() is stored in `answer` like so:
        //  [
        //    { [0] = "UPS", [1] = <UPS name>, [2] = <UPS description> },
        //    { [0] = "UPS", [1] = <UPS name>, [2] = <UPS description> },
        //    { [0] = "END", [1] = "LIST", [2] = "UPS" },
        //  ]
        rc = upscli_list_next(&ups1, LENGTHOF(query), query, &numa, (char***)&answer);
        if (unlikely(-1 == rc)) {
            netdata_log_error("failed to list UPSes from upsd: %s", upscli_upserror(&ups1));
            return NETDATA_PLUGIN_EXIT_AND_DISABLE;
        }

        // Unfortunately, upscli_list_next() will inform us of the end of the list
        // only AFTER it has processed and returned the {"END","LIST","UPS"} entry.
        // That entry could be confusing, and could mistakenly register a UPS
        // named "LIST", so let's skip processing on that item.
        if (streq("END", answer[0][0]))
            break;

        register_ups(answer[0][1]);
    }

    time_t started_t = now_monotonic_sec();

    heartbeat_t hb;
    heartbeat_init(&hb, netdata_update_every * USEC_PER_SEC);
    for (;;) {
        usec_t dt = heartbeat_next(&hb);

        if (unlikely(exit_initiated_get()))
            break;

        rc = upscli_list_start(&ups1, LENGTHOF(query), query);
        if (unlikely(-1 == rc)) {
            netdata_log_error("failed to list UPSes from upsd: %s", upscli_upserror(&ups1));
            return NETDATA_PLUGIN_EXIT_AND_DISABLE;
        }

        for (;;) {
            rc = upscli_list_next(&ups1, LENGTHOF(query), query, &numa, (char***)&answer);
            if (unlikely(-1 == rc)) {
                netdata_log_error("failed to list UPSes from upsd: %s", upscli_upserror(&ups1));
                return NETDATA_PLUGIN_EXIT_AND_DISABLE;
            }

            if (streq("END", answer[0][0]))
                break;

            char *ups_name = answer[0][1];
            char *clean_ups_name = dictionary_get(nd_ups_name, ups_name);
            if (!clean_ups_name) {
                register_ups(ups_name);
                clean_ups_name = dictionary_get(nd_ups_name, ups_name);
            }

            // Track this UPS for future data collection.
            dictionary_set(nd_ups_seen, ups_name, &(bool){true}, sizeof(bool));

            // The 'ups.status' variable is a special case, because its chart has more
            // than one dimension. So, we can't simply print one data point.
            send_metric_ups_status(ups_name, clean_ups_name, dt);

            // The 'ups.realpower' variable is another special case, because if it is
            // not available, then it can be calculated from the ups.load and
            // ups.realpower.nominal variables.
            send_metric_ups_realpower(ups_name, clean_ups_name, dt);

            DICTIONARY *ups_vars = dictionary_get(nd_ups_vars, ups_name);
            dfe_start_read(ups_vars, chart) {
                const char *value = nut_get_var(&ups2, ups_name, chart->nut_variable);
                NETDATA_DOUBLE nut_value_as_num = str2ndd(value, NULL) * NETDATA_PLUGIN_PRECISION;
                send_BEGIN(clean_ups_name, chart->chart_id, dt);
                send_SET(chart->chart_dimension, nut_value_as_num);
                send_END();
            }
            dfe_done(chart);
        }

        // stdout, stderr are connected to pipes.
        // So, if they are closed then netdata must have exited.
        // Flush the data out of the stream buffer to ensure netdata gets it immediately.
        fflush(stdout);
        if (ferror(stdout) && errno == EPIPE) {
            netdata_log_error("failed to fflush(3) upsd data: %s", strerror(errno));
            return NETDATA_PLUGIN_EXIT_AND_DISABLE;
        }

        if (unlikely(exit_initiated_get()))
            break;

        // restart check (14400 seconds)
        if (unlikely(now_monotonic_sec() - started_t > 14400))
            break;

        delete_unseen_ups();
    }

    upscli_disconnect(&ups1);
    upscli_disconnect(&ups2);
    upscli_cleanup();

    return NETDATA_PLUGIN_EXIT_AND_RESTART;
}