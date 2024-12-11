// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_CALCULATED_NUMBER_H
#define NETDATA_ML_CALCULATED_NUMBER_H

#include "dlib/dlib/matrix.h"

// CentOS 7 shenanigans
#include <cmath>
using std::isfinite;

typedef double calculated_number_t;
typedef dlib::matrix<calculated_number_t, 6, 1> DSample;

#endif /* NETDATA_ML_CALCULATED_NUMBER_H */
