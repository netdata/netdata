// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PROC_PRESSURE_H
#define NETDATA_PROC_PRESSURE_H

#define PRESSURE_NUM_RESOURCES 3

struct pressure {
    int updated;
    char *filename;

    struct pressure_chart {
        int enabled;

        const char *id;
        const char *title;

        double value10;
        double value60;
        double value300;

        RRDSET *st;
        RRDDIM *rd10;
        RRDDIM *rd60;
        RRDDIM *rd300;
    } some, full;
};

extern void update_pressure_chart(struct pressure_chart *chart);

#endif //NETDATA_PROC_PRESSURE_H
