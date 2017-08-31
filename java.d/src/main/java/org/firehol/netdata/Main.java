/*
 * Copyright (C) 2017 Simon Nagl
 *
 * netadata-plugin-java-daemon is free software: you can redistribute it and/or modify
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
import org.firehol.netdata.module.jmx.JmxPlugin;
import org.firehol.netdata.plugin.Plugin;
import org.firehol.netdata.plugin.Printer;
import org.firehol.netdata.plugin.configuration.ConfigurationService;
import org.firehol.netdata.utils.LoggingUtils;

public final class Main {
	private static final Logger log = Logger.getLogger("org.firehol.netdata.plugin");

	private static List<Module> modules = Collections.emptyList();

	private Main() {
	}

	public static void main(final String[] args) {
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
		modules = new LinkedList<>();
		modules.add(new JmxPlugin(configService));
	}

	public static void exit(String info) {
		log.severe(info);
		Printer.disable();
		System.exit(1);
	}
}
