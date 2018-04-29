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

import java.io.File;
import java.nio.file.Path;
import java.util.logging.Logger;

import org.firehol.netdata.Main;
import org.firehol.netdata.plugin.configuration.exception.ConfigurationSchemeInstantiationException;
import org.firehol.netdata.plugin.configuration.schema.PluginDaemonConfiguration;
import org.firehol.netdata.utils.logging.LoggingUtils;
import org.firehol.netdata.utils.logging.NetdataLevel;

import com.fasterxml.jackson.core.JsonParseException;
import com.fasterxml.jackson.core.JsonParser.Feature;
import com.fasterxml.jackson.databind.JsonMappingException;
import com.fasterxml.jackson.databind.ObjectMapper;

import lombok.Getter;

public final class ConfigurationService {
	private final Logger log = Logger.getLogger("org.firehol.netdata.plugin.configuration");

	private final ObjectMapper mapper;

	private EnvironmentConfigurationService environmentConfigurationService = EnvironmentConfigurationService
			.getInstance();

	@Getter
	private PluginDaemonConfiguration globalConfiguration;

	private static final ConfigurationService INSTANCE = new ConfigurationService();

	private ConfigurationService() {
		log.fine("Initialize object mapper for reading configuration files.");
		mapper = buildObjectMapper();

		log.info("Read configuration");
		try {
			this.globalConfiguration = readGlobalConfiguration();
		} catch (ConfigurationSchemeInstantiationException e) {
			Main.exit(LoggingUtils.buildMessage("Could not initialize ConfigurationService", e));
		}
	}

	private ObjectMapper buildObjectMapper() {
		ObjectMapper mapper = new ObjectMapper();
		mapper.enable(Feature.ALLOW_COMMENTS);
		return mapper;
	}

	public static ConfigurationService getInstance() {
		return INSTANCE;
	}

	/**
	 * Read a configuration file.
	 * 
	 * <p>
	 * If the file cannot be parsed for some reason this methods tries to use a
	 * default configuration. This is the default instance of the configuration
	 * scheme.
	 * </p>
	 * 
	 * @param <T>
	 *            Configuration Schema.
	 * 
	 * @param file
	 *            to read
	 * @param clazz
	 *            The schema of the configuration.
	 * @return The configuration read from file, or if it was invalid the
	 *         default configuration.
	 * @throws ConfigurationSchemeInstantiationException
	 *             if it was not possible to instantiate clazz
	 */
	protected <T> T readConfiguration(File file, Class<T> clazz) throws ConfigurationSchemeInstantiationException {
		T configuration = null;

		// First try to read the value.
		try {
			configuration = mapper.readValue(file, clazz);
		} catch (JsonParseException | JsonMappingException e) {
			log.log(NetdataLevel.ERROR, LoggingUtils.getMessageSupplier(
					"Could not read malformed configuration file '" + file.getAbsolutePath() + ".", e));
		} catch (Exception e) {
			log.log(NetdataLevel.ERROR, LoggingUtils.getMessageSupplier("Could not read configuration file '"
					+ file.getAbsolutePath() + "', " + clazz + ", " + mapper + ".", e));
		}

		// If we still have no configuration try to read the default one.
		if (configuration == null) {
			try {
				configuration = clazz.newInstance();
			} catch (InstantiationException | IllegalAccessException e) {
				throw new ConfigurationSchemeInstantiationException(
						"Could not instanciate default configuration for class " + clazz.getName() + ".", e);
			}
		}

		return configuration;
	}

	public PluginDaemonConfiguration readGlobalConfiguration() throws ConfigurationSchemeInstantiationException {
		final Path configDir = environmentConfigurationService.getConfigDir();
		Path globalConfigPath = configDir.resolve("java.d.conf");
		PluginDaemonConfiguration globalConfig;

		globalConfig = readConfiguration(globalConfigPath.toFile(), PluginDaemonConfiguration.class);
		return globalConfig;
	}

	public <T> T readModuleConfiguration(String pluginName, Class<T> clazz)
			throws ConfigurationSchemeInstantiationException {
		Path configDir = environmentConfigurationService.getConfigDir().resolve("java.d");
		Path configFile = configDir.resolve(pluginName + ".conf");

		log.info("Reading '" + pluginName + "' module configuration file '" + configFile.toFile().getAbsolutePath()
				+ "'");
		return this.readConfiguration(configFile.toFile(), clazz);
	}
}
