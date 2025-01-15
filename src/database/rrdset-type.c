// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset-type.h"

RRDSET_TYPE rrdset_type_id(const char *name) {
    if(unlikely(strcmp(name, RRDSET_TYPE_AREA_NAME) == 0))
        return RRDSET_TYPE_AREA;

    else if(unlikely(strcmp(name, RRDSET_TYPE_STACKED_NAME) == 0))
        return RRDSET_TYPE_STACKED;

    else if(unlikely(strcmp(name, RRDSET_TYPE_HEATMAP_NAME) == 0))
        return RRDSET_TYPE_HEATMAP;

    else // if(unlikely(strcmp(name, RRDSET_TYPE_LINE_NAME) == 0))
        return RRDSET_TYPE_LINE;
}

const char *rrdset_type_name(RRDSET_TYPE chart_type) {
    switch(chart_type) {
        case RRDSET_TYPE_LINE:
        default:
            return RRDSET_TYPE_LINE_NAME;

        case RRDSET_TYPE_AREA:
            return RRDSET_TYPE_AREA_NAME;

        case RRDSET_TYPE_STACKED:
            return RRDSET_TYPE_STACKED_NAME;

        case RRDSET_TYPE_HEATMAP:
            return RRDSET_TYPE_HEATMAP_NAME;
    }
}
