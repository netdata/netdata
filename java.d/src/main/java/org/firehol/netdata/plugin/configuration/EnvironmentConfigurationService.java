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
import org.firehol.netdata.utils.StringUtils;
import org.firehol.netdata.utils.logging.LoggingUtils;

import lombok.AccessLevel;
import lombok.Getter;

@Getter
public class EnvironmentConfigurationService {
	@Getter(AccessLevel.NONE)
	private final Logger log = Logger.getLogger("org.firehol.netdata.daemon.configuration.environment");

	private Path configDir;

	private Long debugFlags;

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

	/**
	 * @see <a href=
	 *      "https://github.com/firehol/netdata/blob/master/src/log.h">log.h</a>
	 */
	public static final long D_PLUGINSD = 0x0000000000000800;

	/**
	 * @see <a href=
	 *      "https://github.com/firehol/netdata/blob/master/src/log.h">log.h</a>
	 */
	public static final long D_CHILDS = 0x0000000000001000;

	private void readEnvironmentVariables() throws EnvironmentConfigurationException {
		configDir = readNetdataConfigDir();
		debugFlags = readNetdataDebugFlags();
	}

	/**
	 * @see <a href=
	 *      "https://github.com/firehol/netdata/wiki/Tracing-Options">Tracing
	 *      Options</a>
	 */
	private Long readNetdataDebugFlags() throws EnvironmentConfigurationException {
		try {
			String debugFlags = System.getenv("NETDATA_DEBUG_FLAGS");
			if (StringUtils.isBlank(debugFlags))
				return null;
			return Long.decode(debugFlags);
		} catch (RuntimeException e) {
			throw new EnvironmentConfigurationException(
					LoggingUtils.buildMessage("Failed to read 'NETDATA_DEBUG_FLAGS'", e));
		}
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

	/**
	 * Checks whether all given environment Netdata tracing flags are set.
	 * 
	 * @see <a href=
	 *      "https://github.com/firehol/netdata/wiki/Tracing-Options">Tracing
	 *      Options</a>
	 */
	public boolean isDebugFlagSet(long flags) {
		if (getDebugFlags() == null)
			return flags == 0;
		return (getDebugFlags() & flags) == flags;
	}

	/** @see #isDebugFlagSet(long) */
	public boolean isPluginDebugFlagSet() {
		return isDebugFlagSet(EnvironmentConfigurationService.D_PLUGINSD)
				|| isDebugFlagSet(EnvironmentConfigurationService.D_CHILDS);
	}

	/** @deprecated visible for testing only */
	// @VisibleForTesting
	@Deprecated
	static void reload() throws EnvironmentConfigurationException { // NOPMD
		getInstance().readEnvironmentVariables();
	}
}
