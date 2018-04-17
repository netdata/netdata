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

package org.firehol.netdata;

import java.util.Collections;
import java.util.LinkedList;
import java.util.List;
import java.util.logging.Logger;

import org.firehol.netdata.exception.UnreachableCodeException;
import org.firehol.netdata.module.Module;
import org.firehol.netdata.module.jmx.JmxModule;
import org.firehol.netdata.plugin.Plugin;
import org.firehol.netdata.plugin.Printer;
import org.firehol.netdata.plugin.configuration.ConfigurationService;
import org.firehol.netdata.utils.LoggingUtils;
import org.firehol.netdata.utils.NetdataLevel;
import org.firehol.netdata.utils.StringUtils;

/** @see <a href="https://github.com/firehol/netdata/wiki/External-Plugins#native-netdata-plugin-api">External-Plugins: native netdata plugin API</a> */
public final class Main {
	private static final Logger log = Logger.getLogger("org.firehol.netdata.plugin");


	/** @see <a href="https://github.com/firehol/netdata/wiki/Tracing-Options">Tracing Options</a> */
	private static final String NETDATA_DEBUG_FLAGS = "NETDATA_DEBUG_FLAGS";

	/** @see <a href="https://github.com/firehol/netdata/blob/master/src/log.h">log.h</a> */
	private static final long D_PLUGINSD = 0x0000000000000800;

	private static List<Module> modules = Collections.emptyList();

	private Main() {
	}

	/** @see <a href="https://github.com/firehol/netdata/wiki/External-Plugins#command-line-parameters">External-Plugins: command line parameters</a> */
	public static void main(final String[] args) {
		// allow increasing verbosity before reading configuration option
		// org.firehol.netdata.plugin.configuration.schema.PluginDaemonConfiguration#logFullStackTraces
		LoggingUtils.LOG_TRACES = isNetdataPluginDebugEnabled();

		// handle updateEvery parameter provided by netdata
		int updateEverySecond = getUpdateEveryInSecondsFomCommandLineFailFast(args);

		configureModules();
		new Plugin(updateEverySecond, modules).start();
	}

	/**
	 * Chaecks whether the environment Netdata tracing flag for {@code plugins.d} is set.
	 * 
	 * @see <a href="https://github.com/firehol/netdata/wiki/Tracing-Options">Tracing Options</a>
	 */
	public static boolean isNetdataPluginDebugEnabled() {
		try {
			String debugFlags = System.getenv(NETDATA_DEBUG_FLAGS);
			if (StringUtils.isBlank(debugFlags)) debugFlags = System.getProperty(NETDATA_DEBUG_FLAGS); // for testing
			if (StringUtils.isBlank(debugFlags)) return false;
			return (Long.decode(debugFlags) & D_PLUGINSD) != 0;
		} catch (RuntimeException e) {
			log.log(NetdataLevel.ERROR, LoggingUtils.getMessageSupplier("Failed to check " + NETDATA_DEBUG_FLAGS, e));
			return false;
		}
	}

	protected static int getUpdateEveryInSecondsFomCommandLineFailFast(final String[] args) {
		try {
			return new CommandLineArgs(args).getUpdateEveryInSeconds();
		} catch (Exception failureReason) {
			exit(LoggingUtils.buildMessage("Invalid command line options supplied.", failureReason));
			throw new UnreachableCodeException();
		}
	}

	private static void configureModules() {
		ConfigurationService configService = ConfigurationService.getInstance();

		// apply global configuration
		LoggingUtils.LOG_TRACES = isNetdataPluginDebugEnabled() || configService.getGlobalConfiguration().getLogFullStackTraces() == Boolean.TRUE;

		modules = new LinkedList<>();
		modules.add(new JmxModule(configService));
	}

	public static void exit(String info) {
		log.log(NetdataLevel.FATAL, info);
		Printer.disable();
		System.exit(1);
	}
}
