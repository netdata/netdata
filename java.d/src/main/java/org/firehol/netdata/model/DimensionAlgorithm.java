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

package org.firehol.netdata.model;

public enum DimensionAlgorithm {
	/**
	 * the value is to drawn as-is (interpolated to second boundary), if algorithm
	 * is empty, invalid or missing, absolute is used
	 */
	ABSOLUTE,
	/**
	 * the value increases over time, the difference from the last value is
	 * presented in the chart, the server interpolates the value and calculates a
	 * per second figure
	 */
	INCREMENTAL,
	/**
	 * the % of this value compared to the total of all dimensions
	 */
	PERCENTAGE_OF_ABSOLUTE_ROW,
	/**
	 * 
	 */
	PERCENTAGE_OF_INCREMENTAL_ROW;

	@Override
	public String toString() {
		return name().replace('_', '-').toLowerCase();
	}
}
