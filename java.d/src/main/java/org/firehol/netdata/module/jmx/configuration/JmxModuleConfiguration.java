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

package org.firehol.netdata.module.jmx.configuration;

import java.util.ArrayList;
import java.util.List;

import lombok.Getter;
import lombok.Setter;

/**
 * Configuration scheme to configure
 * {@link org.firehol.netdata.module.jmx.JmxModule}
 * 
 * @since 1.0.0
 * @author Simon Nagl
 */
@Getter
@Setter
public class JmxModuleConfiguration {

	/**
	 * If true auto detect and monitor running local virtual machines on plugin
	 * start.
	 */
	private boolean autoDetectLocalVirtualMachines = true;

	/**
	 * A list of JMX servers to monitor.
	 */
	private List<JmxServerConfiguration> jmxServers = new ArrayList<>();

	/**
	 * A list of chart configurations.
	 * 
	 * <p>
	 * Every monitored JMX Servers tries to monitor each chart in this list. If a
	 * JMX Server does not have the required M(X)Beans we won't try adding it over
	 * and over again.
	 * </p>
	 */
	private List<JmxChartConfiguration> commonCharts = new ArrayList<>();
}
