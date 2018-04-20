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

import java.util.ArrayList;
import java.util.List;

import lombok.AccessLevel;
import lombok.Getter;
import lombok.NoArgsConstructor;
import lombok.Setter;

@Getter
@Setter
@NoArgsConstructor
public class Chart {
	/**
	 * Controls the menu the charts will appear in.
	 */
	private String type;
	/**
	 * type.id uniquely identifies the chart, this is what will be needed to add
	 * values to the chart
	 */
	private String id;
	/**
	 * is the name that will be presented to the user instead of id in type.id.
	 * This means that only the id part of type.id is changed. When a name has
	 * been given, the chart is index (and can be referred) as both type.id and
	 * type.name. You can set name to '', or null, or (null) to disable it.
	 */
	private String name;
	/**
	 * the text above the chart
	 */
	private String title;
	/**
	 * the label of the vertical axis of the chart, all dimensions added to a
	 * chart should have the same units of measurement
	 */
	private String units;
	/**
	 * is used to group charts together (for example all eth0 charts should say:
	 * eth0), if empty or missing, the id part of type.id will be used
	 *
	 * this controls the sub-menu on the dashboard
	 */
	private String family;
	/**
	 * The context is giving the template of the chart. For example, if multiple
	 * charts present the same information for a different family, they should
	 * have the same context.
	 *
	 * This is used for looking up rendering information for the chart (colors,
	 * sizes, informational texts) and also apply alarms to it.
	 */
	private String context;
	private ChartType chartType = ChartType.LINE;
	/**
	 * the relative priority of the charts as rendered on the web page. Lower
	 * numbers make the charts appear before the ones with higher numbers.
	 */
	private int priority = 1000;
	/**
	 * overwrite the update frequency set by the server, if empty or missing,
	 * the user configured value will be used
	 */
	private Integer updateEvery;

	@Setter(AccessLevel.NONE)
	private final List<Dimension> allDimension = new ArrayList<>();

	public boolean hasName() {
		return getName() != null;
	}

	public boolean hasFamily() {
		return getFamily() != null;
	}

	public boolean hasContext() {
		return getContext() != null;
	}

	public boolean hasUpdateEvery() {
		return getUpdateEvery() != null;
	}
}
