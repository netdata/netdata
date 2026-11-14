// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_CALCULATED_NUMBER_H
#define NETDATA_ML_CALCULATED_NUMBER_H

#include <cstddef>

// CentOS 7 shenanigans
#include <cmath>
using std::isfinite;

typedef double calculated_number_t;

// One feature vector: the current value and up to 5 lagged values
// (lag_n <= 5). Rows past lag_n MUST be zero: training and scoring use all 6.
struct DSample {
    calculated_number_t v[6] = {};

    static constexpr size_t size() { return 6; }

    calculated_number_t &operator()(size_t i) { return v[i]; }
    const calculated_number_t &operator()(size_t i) const { return v[i]; }

    calculated_number_t *begin() { return v; }
    calculated_number_t *end() { return v + size(); }
    const calculated_number_t *begin() const { return v; }
    const calculated_number_t *end() const { return v + size(); }

    void fill(calculated_number_t value)
    {
        for (calculated_number_t &x : v)
            x = value;
    }
};

static_assert(sizeof(DSample) == 6 * sizeof(calculated_number_t), "DSample must stay 6 packed doubles");

#endif /* NETDATA_ML_CALCULATED_NUMBER_H */
