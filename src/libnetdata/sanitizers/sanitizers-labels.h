// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SANITIZERS_LABELS_H
#define NETDATA_SANITIZERS_LABELS_H

#include "../libnetdata.h"

size_t rrdlabels_sanitize_name(char *dst, const char *src, size_t dst_size);
size_t rrdlabels_sanitize_value(char *dst, const char *src, size_t dst_size);

size_t prometheus_rrdlabels_sanitize_name(char *dst, const char *src, size_t dst_size);

#endif //NETDATA_SANITIZERS_LABELS_H
