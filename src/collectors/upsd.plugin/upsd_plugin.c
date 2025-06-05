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

#define NETDATA_PLUGIN_NAME "upsd"

// https://learn.netdata.cloud/docs/developer-and-contributor-corner/external-plugins#operation
#define NETDATA_PLUGIN_EXIT_AND_RESTART 0
// https://learn.netdata.cloud/docs/developer-and-contributor-corner/external-plugins#disable
#define NETDATA_PLUGIN_EXIT_AND_DISABLE 1

// https://learn.netdata.cloud/docs/developer-and-contributor-corner/external-plugins/#clabel
#define NETDATA_PLUGIN_CLABEL_SOURCE_AUTO   1
#define NETDATA_PLUGIN_CLABEL_SOURCE_MANUAL 2
#define NETDATA_PLUGIN_CLABEL_SOURCE_K8     4
#define NETDATA_PLUGIN_CLABEL_SOURCE_AGENT  8

#define NETDATA_PLUGIN_PRECISION 100

#define BUFLEN 64
#define LENGTHOF(arr) (sizeof(arr)/sizeof(arr[0]))

static bool debug = false;
static unsigned long netdata_update_every = 1;

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
    const size_t chart_dimlength;
    const char *chart_dimension[15];
};

const struct nd_chart nd_charts[] = {
    {
        .nut_variable = "ups.load",
        .chart_id = "load_percentage",
        .chart_title = "UPS load",
        .chart_units = "percentage",
        .chart_family = "ups",
        .chart_context = "upsd.ups_load",
        .chart_type = "area",
        .chart_priority = 70000,
        .chart_dimlength = 1,
        .chart_dimension = { "load" },
    },
    {
        .nut_variable = "ups.realpower",
        .chart_id = "load_usage",
        .chart_title = "UPS load usage (power output)",
        .chart_units = "Watts",
        .chart_family = "ups",
        .chart_context = "upsd.ups_load_usage",
        .chart_type = "line",
        .chart_priority = 70001,
        .chart_dimlength = 1,
        .chart_dimension = { "load_usage" },
    },
    {
        .nut_variable = "ups.status",
        .chart_id = "status",
        .chart_title = "UPS status",
        .chart_units = "status",
        .chart_family = "ups",
        .chart_context = "upsd.ups_status",
        .chart_type = "line",
        .chart_priority = 70002,
        .chart_dimlength = 15,
        .chart_dimension = {
            "on_line",
            "on_battery",
            "low_battery",
            "high_battery",
            "replace_battery",
            "charging",
            "discharging",
            "bypass",
            "calibration",
            "offline",
            "overloaded",
            "trim_input_voltage",
            "boost_input_voltage",
            "forced_shutdown",
            "other",
        },
    },
    {
        .nut_variable = "ups.temperature",
        .chart_id = "temperature",
        .chart_title = "UPS temperature",
        .chart_units = "Celsius",
        .chart_family = "ups",
        .chart_context = "upsd.ups_temperature",
        .chart_type = "line",
        .chart_priority = 70003,
        .chart_dimlength = 1,
        .chart_dimension = { "temperature" },
    },
    {
        .nut_variable = "battery.charge",
        .chart_id = "battery_charge_percentage",
        .chart_title = "UPS Battery charge",
        .chart_units = "percentage",
        .chart_family = "battery",
        .chart_context = "upsd.ups_battery_charge",
        .chart_type = "area",
        .chart_priority = 70004,
        .chart_dimlength = 1,
        .chart_dimension = { "charge" },
    },
    {
        .nut_variable = "battery.runtime",
        .chart_id = "battery_estimated_runtime",
        .chart_title = "UPS Battery estimated runtime",
        .chart_units = "seconds",
        .chart_family = "battery",
        .chart_context = "upsd.ups_battery_estimated_runtime",
        .chart_type = "line",
        .chart_priority = 70005,
        .chart_dimlength = 1,
        .chart_dimension = { "runtime" },
    },
    {
        .nut_variable = "battery.voltage",
        .chart_id = "battery_voltage",
        .chart_title = "UPS Battery voltage",
        .chart_units = "Volts",
        .chart_family = "battery",
        .chart_context = "upsd.ups_battery_voltage",
        .chart_type = "line",
        .chart_priority = 70006,
        .chart_dimlength = 1,
        .chart_dimension = { "voltage" },
    },
    {
        .nut_variable = "battery.voltage.nominal",
        .chart_id = "battery_voltage_nominal",
        .chart_title = "UPS Battery voltage nominal",
        .chart_units = "Volts",
        .chart_family = "battery",
        .chart_context = "upsd.ups_battery_voltage_nominal",
        .chart_type = "line",
        .chart_priority = 70007,
        .chart_dimlength = 1,
        .chart_dimension = { "nominal_voltage" },
    },
    {
        .nut_variable = "input.voltage",
        .chart_id = "input_voltage",
        .chart_title = "UPS Input voltage",
        .chart_units = "Volts",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_voltage",
        .chart_type = "line",
        .chart_priority = 70008,
        .chart_dimlength = 1,
        .chart_dimension = { "voltage" },
    },
    {
        .nut_variable = "input.voltage.nominal",
        .chart_id = "input_voltage_nominal",
        .chart_title = "UPS Input voltage nominal",
        .chart_units = "Volts",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_voltage_nominal",
        .chart_type = "line",
        .chart_priority = 70009,
        .chart_dimlength = 1,
        .chart_dimension = { "nominal_voltage" },
    },
    {
        .nut_variable = "input.current",
        .chart_id = "input_current",
        .chart_title = "UPS Input current",
        .chart_units = "Ampere",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_current",
        .chart_type = "line",
        .chart_priority = 70010,
        .chart_dimlength = 1,
        .chart_dimension = { "current" },
    },
    {
        .nut_variable = "input.current.nominal",
        .chart_id = "input_current_nominal",
        .chart_title = "UPS Input current nominal",
        .chart_units = "Ampere",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_current_nominal",
        .chart_type = "line",
        .chart_priority = 70011,
        .chart_dimlength = 1,
        .chart_dimension = { "nominal_current" },
    },
    {
        .nut_variable = "input.frequency",
        .chart_id = "input_frequency",
        .chart_title = "UPS Input frequency",
        .chart_units = "Hz",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_frequency",
        .chart_type = "line",
        .chart_priority = 70012,
        .chart_dimlength = 1,
        .chart_dimension = { "frequency" },
    },
    {
        .nut_variable = "input.frequency.nominal",
        .chart_id = "input_frequency_nominal",
        .chart_title = "UPS Input frequency nominal",
        .chart_units = "Hz",
        .chart_family = "input",
        .chart_context = "upsd.ups_input_frequency_nominal",
        .chart_type = "line",
        .chart_priority = 70013,
        .chart_dimlength = 1,
        .chart_dimension = { "nominal_frequency" },
    },
    {
        .nut_variable = "output.voltage",
        .chart_id = "output_voltage",
        .chart_title = "UPS Output voltage",
        .chart_units = "Volts",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_voltage",
        .chart_type = "line",
        .chart_priority = 70014,
        .chart_dimlength = 1,
        .chart_dimension = { "voltage" },
    },
    {
        .nut_variable = "output.voltage.nominal",
        .chart_id = "output_voltage_nominal",
        .chart_title = "UPS Output voltage nominal",
        .chart_units = "Volts",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_voltage_nominal",
        .chart_type = "line",
        .chart_priority = 70015,
        .chart_dimlength = 1,
        .chart_dimension = { "nominal_voltage" },
    },
    {
        .nut_variable = "output.current",
        .chart_id = "output_current",
        .chart_title = "UPS Output current",
        .chart_units = "Ampere",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_current",
        .chart_type = "line",
        .chart_priority = 70016,
        .chart_dimlength = 1,
        .chart_dimension = { "current" },
    },
    {
        .nut_variable = "output.current.nominal",
        .chart_id = "output_current_nominal",
        .chart_title = "UPS Output current nominal",
        .chart_units = "Ampere",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_current_nominal",
        .chart_type = "line",
        .chart_priority = 70017,
        .chart_dimlength = 1,
        .chart_dimension = { "nominal_current" },
    },
    {
        .nut_variable = "output.frequency",
        .chart_id = "output_frequency",
        .chart_title = "UPS Output frequency",
        .chart_units = "Hz",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_frequency",
        .chart_type = "line",
        .chart_priority = 70018,
        .chart_dimlength = 1,
        .chart_dimension = { "frequency" },
    },
    {
        .nut_variable = "output.frequency.nominal",
        .chart_id = "output_frequency_nominal",
        .chart_title = "UPS Output frequency nominal",
        .chart_units = "Hz",
        .chart_family = "output",
        .chart_context = "upsd.ups_output_frequency_nominal",
        .chart_type = "line",
        .chart_priority = 70019,
        .chart_dimlength = 1,
        .chart_dimension = { "nominal_frequency" },
    },
    { 0 },
};

void print_version()
{
    fputs("netdata " NETDATA_PLUGIN_NAME ".plugin " NETDATA_VERSION "\n"
          "\n"
          "Copyright 2025 Netdata Inc.\n"
          "Original Author: Mario Campos <mario.andres.campos@gmail.com>\n"
          "Released under GNU General Public License v3+.\n"
          "\n"
          "This program is a data collector plugin for netdata.\n",
          stderr
    );
}

void print_help() {
    fputs("usage: upsd.plugin [-d] COLLECTION_FREQUENCY\n"
          "       upsd.plugin -v\n"
          "       upsd.plugin -h\n"
          "\n"
          "options:\n"
          "  COLLECTION_FREQUENCY    data collection frequency in seconds\n"
          "  -d                      enable verbose output (default: disabled)\n"
          "  -v                      print version and exit\n"
          "  -h                      print this message and exit\n",
          stderr
    );
}

// Netdata will call the plugin with just one command line parameter: the number of
// seconds the user requested this plugin to update its data (by default is also 1).
void parse_command_line(int argc, char *argv[]) {
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

    if (optind >= argc || !isdigit(argv[optind])) {
        print_help();
        exit(EXIT_FAILURE);
    }

    netdata_update_every = str2i(argv[optind]);
    if (netdata_update_every <= 0 || netdata_update_every >= 86400) {
        fputs("COLLECTION_FREQUENCY argument must be between [1,86400)", stderr);
        exit(EXIT_FAILURE);
    }
}

char *clean_name(char *buf, size_t bufsize, const char *name) {
    assert(buf);
    assert(name);

    for (size_t i = 0; i < bufsize; i++) {
        buf[i] = (name[i] == ' ' || name[i] == '.') ? '_': name[i];
        if (name[i] == '\0')
            break;
        if (i+1 == bufsize)
            buf[i] = '\0';
    }
    return buf;
}

const char *nut_get_var(UPSCONN_t *conn, const char *ups_name, const char *var_name) {
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

int nut_list_ups(UPSCONN_t *ups, size_t *numa, char ***answer) {
    assert(ups);
    assert(numa);
    assert(answer);

    int rc;
    static bool started = false;
    const char *query[] = { "UPS" };

    if (!started) {
        rc = upscli_list_start(ups, LENGTHOF(query), query);
        assert(rc != -1);
        started = true;
    }

    rc = upscli_list_next(ups, LENGTHOF(query), query, numa, answer);
    assert(rc != -1);

    // Unfortunately, list_ups_next() will emit the list delimiter
    // "END LIST UPS" as its last iteration before returning 0. We don't
    // need it, so let's skip processing on that item.
    if (!strcmp("END", answer[0][0])) {
        started = false;
        return 0;
    }

    return 1;
}

// This function parses the 'ups.status' variable and emits the Netdata metrics
// for each status, printing 1 for each set status and 0 otherwise.
static inline void print_ups_status_metrics(const char *ups_name, const char *value) {
    assert(ups_name);
    assert(value);

    struct nut_ups_status status = { 0 };

    for (const char *c = value; *c; c++) {
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
                break;
            }
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
                break;
            }
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
                break;
            }
        default:
            status.OTHER = 1;
            break;
        }
    }

    printf("BEGIN %s_%s.status\n"
           "SET 'on_line' = %u\n"
           "SET 'on_battery' = %u\n"
           "SET 'low_battery' = %u\n"
           "SET 'high_battery' = %u\n"
           "SET 'replace_battery' = %u\n"
           "SET 'charging' = %u\n"
           "SET 'discharging' = %u\n"
           "SET 'bypass' = %u\n"
           "SET 'calibration' = %u\n"
           "SET 'offline' = %u\n"
           "SET 'overloaded' = %u\n"
           "SET 'trim_input_voltage' = %u\n"
           "SET 'boost_input_voltage' = %u\n"
           "SET 'forced_shutdown' = %u\n"
           "SET 'other' = %u\n"
           "END\n",
           NETDATA_PLUGIN_NAME, ups_name,
           status.OL,
           status.OB,
           status.LB,
           status.HB,
           status.RB,
           status.CHRG,
           status.DISCHRG,
           status.BYPASS,
           status.CAL,
           status.OFF,
           status.OVER,
           status.TRIM,
           status.BOOST,
           status.FSD,
           status.OTHER);
}

int main(int argc, char *argv[]) {
    size_t numa;
    char **answer[1];
    UPSCONN_t ups1, ups2;
    char buf[BUFLEN];
    unsigned int first_ups_count = 0;

    nd_log_initialize_for_external_plugins("upsd.plugin");
    netdata_threads_init_for_external_plugins(0);

    parse_command_line(argc, argv);

    // If we fail to initialize libupsclient or connect to a local
    // UPS, then there's nothing more to be done; Netdata should disable
    // this plugin, since it cannot offer any metrics.
    if (-1 == upscli_init(0, NULL, NULL, NULL)) {
        netdata_log_error("failed to initialize libupsclient");
        puts("DISABLE");
        exit(NETDATA_PLUGIN_EXIT_AND_DISABLE);
    }

    // TODO: get address/port from configuration file
    if ((-1 == upscli_connect(&ups1, "127.0.0.1", 3493, 0)) ||
        (-1 == upscli_connect(&ups2, "127.0.0.1", 3493, 0))) {
        upscli_cleanup();
        netdata_log_error("failed to connect to upsd at 127.0.0.1:3493");
        puts("DISABLE");
        exit(NETDATA_PLUGIN_EXIT_AND_DISABLE);
    }

    // Set stdout to block-buffered, to make printf() faster.
    setvbuf(stdout, NULL, _IOFBF, BUFSIZ);

    while (nut_list_ups(&ups1, &numa, (char***)&answer)) {
        first_ups_count++;

        // The output of nut_list_ups() will be something like:
        //  { { [0] = "UPS", [1] = <UPS name>, [2] = <UPS description> } }
        const char *ups_name = answer[0][1];

        for (const struct nd_chart *chart = nd_charts; chart->nut_variable; chart++) {
            const char *nut_value = nut_get_var(&ups2, ups_name, chart->nut_variable);

            // Skip metrics that are not available from the UPS.
            if (!nut_value && strcmp(chart->nut_variable, "ups.realpower"))
                continue;

            // If the UPS does not support the 'ups.realpower' variable, then
            // we can still calculate the load_usage if the 'ups.load' and
            // 'ups.realpower.nominal' variables are available.
            if (!nut_value && !strcmp(chart->nut_variable, "ups.realpower") &&
                (!nut_get_var(&ups2, ups_name, "ups.load") || !nut_get_var(&ups2, ups_name, "ups.realpower.nominal")))
                continue;

            // TODO: do not hardcode update_every
            // CHART type.id name title units [family [context [charttype [priority [update_every [options [plugin [module]]]]]]]]
            printf("CHART '%s_%s.%s' '%s' '%s' '%s' '%s' '%s' '%s' '%u' '%u' '%s' '%s'\n",
                   NETDATA_PLUGIN_NAME, clean_name(buf, sizeof(buf), ups_name), chart->chart_id, // type.id
                   "",                    // name
                   chart->chart_title,    // title
                   chart->chart_units,    // units
                   chart->chart_family,   // family
                   chart->chart_context,  // context
                   chart->chart_type,     // charttype
                   chart->chart_priority, // priority
                   1,                     // update_every
                   "",                    // options
                   NETDATA_PLUGIN_NAME);  // plugin

            if ((nut_value = nut_get_var(&ups2, ups_name, "battery.type")))
                printf("CLABEL 'battery_type' '%s' '%u'\n", nut_value, NETDATA_PLUGIN_CLABEL_SOURCE_AUTO);
            if ((nut_value = nut_get_var(&ups2, ups_name, "device.model")))
                printf("CLABEL 'device_model' '%s' '%u'\n", nut_value, NETDATA_PLUGIN_CLABEL_SOURCE_AUTO);
            if ((nut_value = nut_get_var(&ups2, ups_name, "device.serial")))
                printf("CLABEL 'device_serial' '%s' '%u'\n", nut_value, NETDATA_PLUGIN_CLABEL_SOURCE_AUTO);
            if ((nut_value = nut_get_var(&ups2, ups_name, "device.mfr")))
                printf("CLABEL 'device_manufacturer' '%s' '%u'\n", nut_value, NETDATA_PLUGIN_CLABEL_SOURCE_AUTO);
            if ((nut_value = nut_get_var(&ups2, ups_name, "device.type")))
                printf("CLABEL 'device_type' '%s' '%u'\n", nut_value, NETDATA_PLUGIN_CLABEL_SOURCE_AUTO);

            printf("CLABEL 'ups_name' '%s' '%u'\n", ups_name, NETDATA_PLUGIN_CLABEL_SOURCE_AUTO);
            printf("CLABEL '_collect_plugin' '%s' '%u'\n", NETDATA_PLUGIN_NAME, NETDATA_PLUGIN_CLABEL_SOURCE_AUTO);
            puts("CLABEL_COMMIT");

            for (size_t j = 0; j < chart->chart_dimlength; j++)
                printf("DIMENSION '%s'\n", chart->chart_dimension[j]);
        }
    }

    time_t started_t = now_monotonic_sec();

    heartbeat_t hb;
    heartbeat_init(&hb, netdata_update_every * USEC_PER_SEC);
    for (;;) {
        heartbeat_next(&hb);

        if (unlikely(exit_initiated_get()))
            break;

        unsigned int this_ups_count = 0;
        while (nut_list_ups(&ups1, &numa, (char***)&answer)) {
            this_ups_count++;

            const char *ups_name = answer[0][1];
            const char *clean_ups_name = clean_name(buf, sizeof(buf), ups_name);

            for (const struct nd_chart *chart = nd_charts; chart->nut_variable; chart++) {
                const char *nut_value = nut_get_var(&ups2, ups_name, chart->nut_variable);

                // Skip metrics that are not available from the UPS.
                if (!nut_value && strcmp(chart->nut_variable, "ups.realpower"))
                    continue;

                if (!nut_value && !strcmp(chart->nut_variable, "ups.realpower")) {
                    if (nut_get_var(&ups2, ups_name, "ups.load") &&
                        nut_get_var(&ups2, ups_name, "ups.realpower.nominal")) {
                        double load = atof(nut_get_var(&ups2, ups_name, "ups.load"));
                        double nominal = atof(nut_get_var(&ups2, ups_name, "ups.realpower.nominal"));
                        double load_usage = (load / 100) * nominal * NETDATA_PLUGIN_PRECISION;
                        snprintf(buf, sizeof(buf), "%d", (int)load_usage);
                        nut_value = buf;
                    }
                    else continue;
                }

                // The 'ups.status' variable is a special case, because its chart has more
                // than one dimension. So, we can't simply print one data point.
                if (!strcmp(chart->nut_variable, "ups.status")) {
                    print_ups_status_metrics(clean_ups_name, nut_value);
                    continue;
                }

                printf("BEGIN '%s_%s.%s'\n"
                       "SET '%s' = %s\n"
                       "END\n",
                       NETDATA_PLUGIN_NAME, clean_ups_name, chart->chart_id,
                       chart->chart_dimension[0], nut_value);
            }
        }

        // Flush the data out of the stream buffer to ensure netdata gets it immediately.
        // https://learn.netdata.cloud/docs/developer-and-contributor-corner/external-plugins#the-output-of-the-plugin
        fflush(stdout);

        // stdout, stderr are connected to pipes.
        // So, if they are closed then netdata must have exited.
        if (ferror(stdout) && errno == EPIPE) {
            netdata_log_error("fflush(3)");
            return EXIT_FAILURE;
        }

        // If the last UPS count does not match the current UPS count, then there's a real
        // chance that our UPS information is outdated; restart this plugin to get accurate UPSes.
        if (unlikely(first_ups_count != this_ups_count)) {
            netdata_log_error("Detected change in UPSes (count: %u -> %u); restarting to read UPS data.",
                first_ups_count, this_ups_count);
            break;
        }

        if (unlikely(exit_initiated_get()))
            break;

        // restart check (14400 seconds)
        if (unlikely(now_monotonic_sec() - started_t > 14400))
            break;
    }

    upscli_disconnect(&ups1);
    upscli_disconnect(&ups2);
    upscli_cleanup();

    return NETDATA_PLUGIN_EXIT_AND_RESTART
}