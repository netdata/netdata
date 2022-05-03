// SPDX-License-Identifier: GPL-3.0

#ifndef KOLMOGOROVSMIRNOVDIST_H
#define KOLMOGOROVSMIRNOVDIST_H

#ifdef __cplusplus
extern "C" {
#endif


/********************************************************************
 *
 * File:          KolmogorovSmirnovDist.h
 * Environment:   ISO C99 or ANSI C89
 * Author:        Richard Simard
 * Organization:  DIRO, Université de Montréal
 * Date:          1 February 2012
 * Version        1.1
 *
 * Copyright March 2010 by Université de Montréal,
                           Richard Simard and Pierre L'Ecuyer
 =====================================================================

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, version 3 of the License.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with this program.  If not, see <http://www.gnu.org/licenses/>.

 =====================================================================*/
/*
 *
 * The Kolmogorov-Smirnov test statistic D_n is defined by
 *
 *        D_n = sup_x |F(x) - S_n(x)|
 *
 * where n is the sample size, F(x) is a completely specified theoretical
 * distribution, and S_n(x) is an empirical distribution function.
 *
 *
 * The function
 *
 *        double KScdf (int n, double x);
 *
 * computes the cumulative probability P[D_n <= x] of the 2-sided 1-sample
 * Kolmogorov-Smirnov distribution with sample size n at x.
 * It returns at least 13 decimal digits of precision for n <= 500,
 * at least 7 decimal digits of precision for 500 < n <= 100000,
 * and a few correct decimal digits for n > 100000.
 *
 */

double KScdf (int n, double x);


/*
 * The function
 *
 *        double KSfbar (int n, double x);
 *
 * computes the complementary cumulative probability P[D_n >= x] of the
 * 2-sided 1-sample Kolmogorov-Smirnov distribution with sample size n at x.
 * It returns at least 10 decimal digits of precision for n <= 500,
 * at least 6 decimal digits of precision for 500 < n <= 200000,
 * and a few correct decimal digits for n > 200000.
 *
 */

double KSfbar (int n, double x);


/*
 * NOTE:
 * The ISO C99 function log1p of the standard math library does not exist in
 * ANSI C89. Here, it is programmed explicitly in KolmogorovSmirnovDist.c.

 * For ANSI C89 compilers, change the preprocessor condition to make it
 * available.
 */

#ifdef __cplusplus
}
#endif

#endif
