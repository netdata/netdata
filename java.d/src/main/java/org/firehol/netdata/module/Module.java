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

import java.util.Collection;

import org.firehol.netdata.exception.InitializationException;
import org.firehol.netdata.model.Chart;
import org.firehol.netdata.model.Dimension;
import org.firehol.netdata.plugin.Collector;
import org.firehol.netdata.plugin.configuration.ConfigurationService;

/**
 * A module of the {@code java.d} netdata plugin.
 */
public interface Module extends Collector {

	/**
	 * Called once to initialize this module before the first call to
	 * {@link #collectValues()}.
	 * 
	 * If the module has a configuration file, it may be loaded as:
	 * 
	 * <pre>
	 * <code>
	 * Collection&ltChart> initialize() throws InitializationException {
	 *     YourConfiguration conf = configurationService.getModuleConfiguration(getName(), YourConfiguration.class);
	 *     // apply conf
	 * }
	 * </code>
	 * </pre>
	 * 
	 * @return the charts with dimensions known at initialization
	 * @throws InitializationException
	 *             if this module could not be initialized, the module will be
	 *             excluded from future processing
	 */
	@Override
	Collection<Chart> initialize() throws InitializationException;

	/**
	 * Called periodically to get the charts with dimensions this module has
	 * collected values for.
	 * 
	 * <p>
	 * Any collected values set via {@link Dimension#setCurrentValue(Long)} will
	 * be cleared before the next call to {@link #collectValues()}.
	 * </p>
	 * 
	 * <p>
	 * Charts and/or dimensions may be added between calls, however their
	 * definitions will show up in netdata as first seen. If charts and
	 * dimensions are static, it is advised to provide them already in
	 * {@link #initialize()}.
	 * </p>
	 * 
	 * <p>
	 * This method will not be called from multiple threads.
	 * </p>
	 * 
	 * <p>
	 * <b> Note: If this method does not complete under {@code updateEvery}
	 * seconds, it may delay data collection of other modules too. </b>
	 * </p>
	 * 
	 * <p>
	 * <b> Note: Any exception thrown will be treated as fatal not only to this
	 * module, but also to the whole {@code java.d} plugin. </b>
	 * </p>
	 */
	@Override
	Collection<Chart> collectValues();

	/**
	 * Release any resources held by this module.
	 */
	@Override
	void cleanup();

	/**
	 * The name of this module as configured by
	 * {@link Builder#build(ConfigurationService, String)}.
	 * 
	 * <p>
	 * The module name is commonly used as a prefix in {@link Chart#type}.
	 * </p>
	 */
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
