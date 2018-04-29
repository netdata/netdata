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

package org.firehol.netdata.module;

import org.firehol.netdata.plugin.Collector;
import org.firehol.netdata.plugin.configuration.ConfigurationService;

public interface Module extends Collector {

	String getName();

	/**
	 * Knows how to construct this module.
	 * 
	 * <p>
	 * <b> Note: Implementations MUST provide a public a no-arg constructor.
	 * </b>
	 * </p>
	 * 
	 * <p>
	 * Implementations will typically invoke
	 * {@link ConfigurationService#readModuleConfiguration(String, Class)} only
	 * later when {@link Module#initialize()} is called.
	 * </p>
	 */
	public static interface Builder {
		public Module build(ConfigurationService configurationService, String moduleName);
	}
}
