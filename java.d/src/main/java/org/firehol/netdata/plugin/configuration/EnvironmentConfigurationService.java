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

package org.firehol.netdata.plugin.configuration;

import java.nio.file.InvalidPathException;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.logging.Logger;

import org.firehol.netdata.Main;
import org.firehol.netdata.plugin.configuration.exception.EnvironmentConfigurationException;
import org.firehol.netdata.utils.LoggingUtils;

import lombok.AccessLevel;
import lombok.Getter;

@Getter
public class EnvironmentConfigurationService {
	@Getter(AccessLevel.NONE)
	private final Logger log = Logger.getLogger("org.firehol.netdata.daemon.configuration.environment");

	private Path configDir;

	private static final EnvironmentConfigurationService INSTANCE = new EnvironmentConfigurationService();

	public static EnvironmentConfigurationService getInstance() {
		return INSTANCE;
	}

	private EnvironmentConfigurationService() {
		try {
			readEnvironmentVariables();
		} catch (EnvironmentConfigurationException e) {
			// Fail fast if this important singleton could not initialize.
			Main.exit(e.getMessage());
		}
	}

	private void readEnvironmentVariables() throws EnvironmentConfigurationException {
		configDir = readNetdataConfigDir();
	}

	protected Path readNetdataConfigDir() throws EnvironmentConfigurationException {
		log.fine("Parse environment variable NETDATA_CONFIG_DIR");
		String configDirString = System.getenv("NETDATA_CONFIG_DIR");

		if (configDirString == null) {
			throw new EnvironmentConfigurationException(
					"Expected environment variable 'NETDATA_CONFIG_DIR' is missing");
		}

		Path configDir;

		try {
			configDir = Paths.get(configDirString);
		} catch (InvalidPathException e) {
			throw new EnvironmentConfigurationException(
					LoggingUtils.buildMessage("NETDATA_CONFIG_DIR contains no valid path name.", e));
		}

		return configDir;

	}
}
