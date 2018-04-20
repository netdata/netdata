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

package org.firehol.netdata.plugin;

import org.firehol.netdata.model.Chart;
import org.firehol.netdata.model.Dimension;

/**
 * The class {@code Printer} contains methods to communicate with the caller.
 * 
 * The format of the communication is defined <a href=
 * "https://github.com/firehol/netdata/wiki/External-Plugins#netdata-plugins">here</a>
 * 
 * @author Simon Nagl
 * @since 1.0.0
 */
public final class Printer {

	private Printer() {
	}

	private static void print(final String command) {
		System.out.println(command);
	}

	public static void initializeChart(final Chart chart) {
		StringBuilder sb = new StringBuilder();
		appendInitializeChart(sb, chart);

		for (Dimension dimension : chart.getAllDimension()) {
			sb.append(System.lineSeparator());
			appendInitializeDimension(sb, dimension);
		}

		print(sb.toString());
	}

	protected static void appendInitializeChart(StringBuilder sb, final Chart chart) {
		// Start new chart
		sb.append("CHART ");
		// Append identifier
		sb.append(chart.getType());
		sb.append('.');
		sb.append(chart.getId());
		sb.append(' ');
		// Append name
		if (chart.hasName()) {
			sb.append(chart.getName());
		} else {
			sb.append("null");
		}
		sb.append(" '");
		// Append title
		sb.append(chart.getTitle());
		sb.append("' ");
		// Append untis
		sb.append(chart.getUnits());
		sb.append(' ');
		// Append familiy
		if (chart.hasFamily()) {
			sb.append(chart.getFamily());
		} else {
			sb.append(chart.getId());
		}
		sb.append(' ');
		// Append context
		if (chart.hasContext()) {
			sb.append(chart.getContext());
		} else {
			sb.append(chart.getId());
		}
		sb.append(' ');
		// Append chart type
		sb.append(chart.getChartType());
		sb.append(' ');
		// Append priority
		sb.append(chart.getPriority());
		// Append update_every
		if (chart.hasUpdateEvery()) {
			sb.append(' ');
			sb.append(chart.getUpdateEvery());
		}
	}

	protected static void appendInitializeDimension(StringBuilder sb, final Dimension dimension) {
		// Start new dimension
		sb.append("DIMENSION ");
		// Append ID
		sb.append(dimension.getId());
		sb.append(' ');
		// Append name
		if (dimension.hasName()) {
			sb.append(dimension.getName());
		} else {
			sb.append(dimension.getId());
		}
		sb.append(' ');
		// Append algorithm
		sb.append(dimension.getAlgorithm());
		sb.append(' ');
		// Append multiplier
		sb.append(dimension.getMultiplier());
		sb.append(' ');
		// Append divisor
		sb.append(dimension.getDivisor());
		// Append hidden
		if (dimension.isHidden()) {
			sb.append(" hidden");
		}
	}

	public static void collect(final Chart chart) {
		StringBuilder sb = new StringBuilder();
		appendCollectBegin(sb, chart);

		for (Dimension dim : chart.getAllDimension()) {
			if (dim.getCurrentValue() != null) {
				sb.append(System.lineSeparator());
				appendCollectDimension(sb, dim);
				dim.setCurrentValue(null);
			}
		}

		sb.append(System.lineSeparator());
		appendCollectEnd(sb);

		print(sb.toString());
	}

	protected static void appendCollectBegin(StringBuilder sb, Chart chart) {
		// TODO Add microseconds to the output.
		sb.append("BEGIN ");
		sb.append(chart.getType());
		sb.append('.');
		sb.append(chart.getId());
	}

	protected static void appendCollectDimension(StringBuilder sb, Dimension dim) {
		sb.append("SET ");
		sb.append(dim.getId());
		sb.append(" = ");
		sb.append(dim.getCurrentValue());
	}

	protected static void appendCollectEnd(StringBuilder sb) {
		sb.append("END");
	}

	/**
	 * Tell the caller to disable the plugin. This will prevent it from
	 * restarting the plugin.
	 */
	public static void disable() {
		print("DISABLE");
	}
}
