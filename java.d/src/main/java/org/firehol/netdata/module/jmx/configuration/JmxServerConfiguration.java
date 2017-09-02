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

import java.util.List;

import com.fasterxml.jackson.annotation.JsonIgnore;

import lombok.Getter;
import lombok.Setter;

/**
 * Configuration scheme to configure JMX agents to monitor.
 */
@Getter
@Setter
public class JmxServerConfiguration {
	/**
	 * JMX Service URL used to connect to the JVM.
	 * 
	 * <blockquote> {@code service:jmx:rmi://[host[:port]][urlPath]} </blockquote>
	 * 
	 * @see <a href=
	 *      "https://docs.oracle.com/cd/E19159-01/819-7758/gcnqf/index.html">Oracle
	 *      Developer's Guide for JMX Clients</a>
	 * 
	 */
	private String serviceUrl;

	/**
	 * Name displayed at the dashboard.
	 */
	private String name;

	@JsonIgnore
	// This property is not part of the configuration scheme.
	// This is a technical property used by the plugin.
	private List<JmxChartConfiguration> charts;
}
