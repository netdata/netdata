// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HW_SENSORS_H
#define NETDATA_HW_SENSORS_H

// --------------------------------------------------------------------------------------------------------------------
// Cross-OS temperature sensors histogram
//
// PERMANENT CONTRACT: every hardware-sensor producer emits this context with
// exactly these buckets, so Netdata Cloud can sum the bucket counts across
// nodes into a fleet-wide thermal distribution view.
//
// Buckets are DISJOINT bands (not cumulative): each dimension counts the
// sensors whose current reading falls inside its band only, and every sensor
// increments exactly one bucket. Dimensions are named after the band's
// EXCLUSIVE upper bound, in degrees Celsius: dimension "40" counts t < 40,
// "50" counts 40 <= t < 50, ..., "100" counts 95 <= t < 100, and "+Inf"
// counts t >= 100. 10C-wide bands through the benign range, 5C-wide bands
// in the 80-100C action zone.
//
// Producers using this header: macos.plugin, windows.plugin.
// Producers mirroring this contract (different emission mechanism - keep in
// sync by hand): debugfs.plugin libsensors (PLUGINSD text protocol).

#define HW_SENSORS_TEMPERATURE_HISTOGRAM_CONTEXT "system.hw.sensor.temperature.histogram"
#define HW_SENSORS_TEMPERATURE_HISTOGRAM_TITLE "Temperature Sensors Distribution"
#define HW_SENSORS_TEMPERATURE_HISTOGRAM_UNITS "sensors"
#define HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS 10

static const int hw_sensors_temperature_histogram_upper_bounds[HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS - 1] = {
    40, 50, 60, 70, 80, 85, 90, 95, 100,
};

static const char *hw_sensors_temperature_histogram_dimensions[HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS] = {
    "40", "50", "60", "70", "80", "85", "90", "95", "100", "+Inf",
};

typedef struct {
    RRDSET *st;
    RRDDIM *rds[HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS];
    uint32_t counts[HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS];
} HW_SENSORS_TEMPERATURE_HISTOGRAM;

static inline void hw_sensors_temperature_histogram_reset(HW_SENSORS_TEMPERATURE_HISTOGRAM *h) {
    memset(h->counts, 0, sizeof(h->counts));
}

static inline void hw_sensors_temperature_histogram_add(HW_SENSORS_TEMPERATURE_HISTOGRAM *h, NETDATA_DOUBLE celsius) {
    // NAN fails every comparison below and would silently land in the first
    // bucket - reject invalid readings here to protect all producers
    if (unlikely(!isfinite(celsius)))
        return;

    size_t i = 0;
    while (i < HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS - 1 &&
           celsius >= (NETDATA_DOUBLE)hw_sensors_temperature_histogram_upper_bounds[i])
        i++;

    h->counts[i]++;
}

static inline void common_hw_sensors_temperature_histogram(
    HW_SENSORS_TEMPERATURE_HISTOGRAM *h,
    int update_every,
    const char *plugin,
    const char *module)
{
    if (unlikely(!h->st)) {
        // do not create the chart before at least one temperature sensor
        // exists - machines without temperature sensors get no chart at all
        uint32_t total = 0;
        for (size_t i = 0; i < HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS; i++)
            total += h->counts[i];
        if (!total)
            return;

        h->st = rrdset_create_localhost(
            "sensors",
            "temperature_histogram",
            NULL,
            "Temperature",
            HW_SENSORS_TEMPERATURE_HISTOGRAM_CONTEXT,
            HW_SENSORS_TEMPERATURE_HISTOGRAM_TITLE,
            HW_SENSORS_TEMPERATURE_HISTOGRAM_UNITS,
            plugin,
            module,
            NETDATA_CHART_PRIO_SENSORS - 10,
            update_every,
            RRDSET_TYPE_HEATMAP);

        for (size_t i = 0; i < HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS; i++)
            h->rds[i] = rrddim_add(
                h->st, hw_sensors_temperature_histogram_dimensions[i], NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    for (size_t i = 0; i < HW_SENSORS_TEMPERATURE_HISTOGRAM_BUCKETS; i++)
        rrddim_set_by_pointer(h->st, h->rds[i], (collected_number)h->counts[i]);

    rrdset_done(h->st);
}

#endif //NETDATA_HW_SENSORS_H
