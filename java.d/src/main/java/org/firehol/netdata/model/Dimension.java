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

import lombok.Getter;
import lombok.NoArgsConstructor;
import lombok.Setter;

@Getter
@Setter
@NoArgsConstructor
public class Dimension {
	/**
	 * Identifier of this dimension (it is a text value, not numeric), this will be
	 * needed later to add values to the dimension.
	 */
	private String id;
	/**
	 * Name of the dimension as it will appear at the legend of the chart, if empty
	 * or missing the id will be used.
	 */
	private String name;
	private DimensionAlgorithm algorithm = DimensionAlgorithm.ABSOLUTE;
	/**
	 * Multiply the collected value.
	 */
	private int multiplier = 1;
	/**
	 * Divide the collected value.
	 */
	private int divisor = 1;
	/**
	 * Make this dimension hidden, it will take part in the calculations but will
	 * not be presented in the chart.
	 */
	private boolean hidden;

	/**
	 * Current collected value. Null if the last value was sent to netdata.
	 */
	private Long currentValue;

	public boolean hasName() {
		return getName() != null;
	}
}
