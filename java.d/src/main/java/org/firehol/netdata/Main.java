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
import org.firehol.netdata.plugin.Plugin;
import org.firehol.netdata.plugin.Printer;
import org.firehol.netdata.plugin.configuration.ConfigurationService;
import org.firehol.netdata.plugin.configuration.EnvironmentConfigurationService;
import org.firehol.netdata.utils.logging.LoggingUtils;
import org.firehol.netdata.utils.logging.NetdataLevel;

/**
 * @see <a href=
 *      "https://github.com/firehol/netdata/wiki/External-Plugins#native-netdata-plugin-api">External-Plugins:
 *      native netdata plugin API</a>
 */
public final class Main {
	private static final Logger log = Logger.getLogger("org.firehol.netdata.plugin");

	private static List<Module> modules = Collections.emptyList();

	private Main() {
	}

	/**
	 * @see <a href=
	 *      "https://github.com/firehol/netdata/wiki/External-Plugins#command-line-parameters">External-Plugins:
	 *      command line parameters</a>
	 */
	public static void main(final String[] args) {
		// allow increasing verbosity before reading configuration option
		// org.firehol.netdata.plugin.configuration.schema.PluginDaemonConfiguration#logFullStackTraces
		LoggingUtils.LOG_TRACES = EnvironmentConfigurationService.getInstance().isPluginDebugFlagSet();

		// handle updateEvery parameter provided by netdata
		int updateEverySecond = getUpdateEveryInSecondsFomCommandLineFailFast(args);

		configureModules();
		new Plugin(updateEverySecond, modules).start();
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
		LoggingUtils.LOG_TRACES = EnvironmentConfigurationService.getInstance().isPluginDebugFlagSet()
				|| configService.getGlobalConfiguration().getLogFullStackTraces() == Boolean.TRUE;

		// instantiate modules via builder
		modules = new LinkedList<>();
		configService.getGlobalConfiguration().getModules().forEach((moduleName, builderName) -> {
			try {
				Module.Builder builder = (Module.Builder) Class.forName(builderName).newInstance();
				modules.add(builder.build(configService, moduleName));
			} catch (Exception e) {
				// TODO: should we go on initializing other modules?
				throw new IllegalArgumentException(
						"Unable to instantiate builder '" + builderName + "' for module: " + moduleName, e);
			}
		});
		if (modules.isEmpty()) {
			exit("No modules to run");
		}
	}

	public static void exit(String info) {
		log.log(NetdataLevel.FATAL, info);
		Printer.disable();
		System.exit(1); // NOPMD we have to notify netdata via exit status
	}
}
