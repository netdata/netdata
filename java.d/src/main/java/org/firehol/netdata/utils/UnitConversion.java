/*
 * Copyright (C) 2017 Simon Nagl
 *
 * netdata is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package org.firehol.netdata.utils;

import java.util.concurrent.TimeUnit;

public abstract class UnitConversion {
	public static long NANO_PER_PLAIN = TimeUnit.SECONDS.toNanos(1);

	public static long MILI_PER_PLAIN = TimeUnit.SECONDS.toMillis(1);

	public static long MILI_PER_NANO = TimeUnit.MILLISECONDS.toNanos(1);
}
