// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PROC_PRESSURE_H
#define NETDATA_PROC_PRESSURE_H

#define PRESSURE_NUM_RESOURCES 4

struct pressure {
    char *filename;
    bool staterr;
    int updated;

    struct pressure_charts {
        bool available;
        int enabled;

        struct pressure_share_time_chart {
            const char *id;
            const char *title;

            double value10;
            double value60;
            double value300;

            RRDSET *st;
            RRDDIM *rd10;
            RRDDIM *rd60;
            RRDDIM *rd300;
        } share_time;

        struct pressure_total_time_chart {
            const char *id;
            const char *title;

            unsigned long long value_total;

            RRDSET *st;
            RRDDIM *rdtotal;
        } total_time;
    } some, full;
};

void update_pressure_charts(struct pressure_charts *charts);

#endif //NETDATA_PROC_PRESSURE_H
