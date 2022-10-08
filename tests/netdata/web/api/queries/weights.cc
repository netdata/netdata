#include "daemon/common.h"
#include <gtest/gtest.h>

/*

    Unit tests against the output of this:

        https://github.com/scipy/scipy/blob/4cf21e753cf937d1c6c2d2a0e372fbc1dbbeea81/scipy/stats/_stats_py.py#L7275-L7449

        import matplotlib.pyplot as plt
        import pandas as pd
        import numpy as np
        import scipy as sp
        from scipy import stats

        data1 = np.array([ 1111, -2222, 33, 100, 100, 15555, -1, 19999, 888, 755, -1, -730 ])
        data2 = np.array([365, -123, 0])
        data1 = np.sort(data1)
        data2 = np.sort(data2)
        n1 = data1.shape[0]
        n2 = data2.shape[0]
        data_all = np.concatenate([data1, data2])
        cdf1 = np.searchsorted(data1, data_all, side='right') / n1
        cdf2 = np.searchsorted(data2, data_all, side='right') / n2
        print(data_all)
        print("\ndata1", data1, cdf1)
        print("\ndata2", data2, cdf2)
        cddiffs = cdf1 - cdf2
        print("\ncddiffs", cddiffs)
        minS = np.clip(-np.min(cddiffs), 0, 1)
        maxS = np.max(cddiffs)
        print("\nmin", minS)
        print("max", maxS)
        m, n = sorted([float(n1), float(n2)], reverse=True)
        en = m * n / (m + n)
        d = max(minS, maxS)
        prob = stats.distributions.kstwo.sf(d, np.round(en))
        print("\nprob", prob)

*/

TEST(weights, ks_2samp) {
    {
        int bs = 3, hs = 3;
        DIFFS_NUMBERS base[3] = { 1, 2, 3 };
        DIFFS_NUMBERS high[3] = { 3, 4, 6 };

        double prob = ks_2samp(base, bs, high, hs, 0);
        EXPECT_NEAR(prob, 0.222222, 1.0e-6);
    }

    {
        int bs = 6, hs = 3;
        DIFFS_NUMBERS base[6] = { 1, 2, 3, 10, 10, 15 };
        DIFFS_NUMBERS high[3] = { 3, 4, 6 };

        double prob = ks_2samp(base, bs, high, hs, 1);
        EXPECT_NEAR(prob, 0.5, 1.0e-6);
    }

    {
        int bs = 12, hs = 3;
        DIFFS_NUMBERS base[12] = { 1, 2, 3, 10, 10, 15, 111, 19999, 8, 55, -1, -73 };
        DIFFS_NUMBERS high[3] = { 3, 4, 6 };

        double prob = ks_2samp(base, bs, high, hs, 2);
        EXPECT_NEAR(prob, 0.347222, 1.0e-6);
    }

    {
        int bs = 12, hs = 3;
        DIFFS_NUMBERS base[12] = { 1111, -2222, 33, 100, 100, 15555, -1, 19999, 888, 755, -1, -730 };
        DIFFS_NUMBERS high[3] = { 365, -123, 0 };

        double prob = ks_2samp(base, bs, high, hs, 2);
        EXPECT_NEAR(prob, 0.777778, 1.0e-6);
    }
}
