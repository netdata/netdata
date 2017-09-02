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

import static org.junit.Assert.assertEquals;

import java.io.File;
import java.io.IOException;
import java.nio.file.Files;

import org.firehol.netdata.plugin.configuration.exception.ConfigurationSchemeInstantiationException;
import org.firehol.netdata.plugin.configuration.schema.PluginDaemonConfiguration;
import org.junit.Before;
import org.junit.Rule;
import org.junit.Test;
import org.junit.contrib.java.lang.system.EnvironmentVariables;
import org.junit.rules.TemporaryFolder;

public class ConfigurationServiceTest {

	// We cannot instantiate ConfigurationService here because it depends on an
	// environment Variable.

	@Rule
	public TemporaryFolder tmpFolder = new TemporaryFolder();

	@Rule
	public final EnvironmentVariables environmentVariables = new EnvironmentVariables();

	@Before
	public void setUp() {
		environmentVariables.set("NETDATA_CONFIG_DIR", tmpFolder.getRoot().toString());
	}

	/**
	 * Test reading valid {@code TestConfiguration}
	 */
	@Test
	public void testReadConfiguration() throws IOException, ConfigurationSchemeInstantiationException {
		// Object under test
		ConfigurationService configService = ConfigurationService.getInstance();

		// Write a TestConfiguration to a temporary file.
		File testConfigurationFile = tmpFolder.newFile();
		Files.write(testConfigurationFile.toPath(), "{ \"testProperty\": \"testValue\" }".getBytes());


		// Test
		TestConfiguration testConfig = configService.readConfiguration(testConfigurationFile,
				TestConfiguration.class);

		// Verify
		assertEquals("testValue", testConfig.testProperty);
	}

	/**
	 * Test reading invalid {@code TestConfiguration}
	 */
	@Test
	public void testReadConfigurationInvalid() throws IOException, ConfigurationSchemeInstantiationException {
		// Object under test
		ConfigurationService configService = ConfigurationService.getInstance();

		// Write a TestConfiguration to a temporary file.
		File testConfigurationFile = tmpFolder.newFile();
		Files.write(testConfigurationFile.toPath(), "{ \"noClassProperty\": \"testValue\" }".getBytes());

		// Test
		TestConfiguration testConfig = configService.readConfiguration(testConfigurationFile, TestConfiguration.class);

		// Verify
		assertEquals("defaultValue", testConfig.testProperty);
	}

	/**
	 * Test reading invalid {@code NoTestConfiguration}
	 */
	@Test(expected = ConfigurationSchemeInstantiationException.class)
	public void testReadConfigurationFailure() throws IOException, ConfigurationSchemeInstantiationException {
		// Object under test
		ConfigurationService configService = ConfigurationService.getInstance();

		// Write a TestConfiguration to a temporary file.
		File testConfigurationFile = tmpFolder.newFile();
		Files.write(testConfigurationFile.toPath(), "{ \"noClassProperty\": \"testValue\" }".getBytes());

		// Test
		NoTestConfiguration testConfig = configService.readConfiguration(testConfigurationFile,
				NoTestConfiguration.class);

		// Verify
		assertEquals("defaultValue", testConfig.testProperty);
	}

	@Test
	public void testReadGlobalConfiguration() throws IOException, ConfigurationSchemeInstantiationException {
		// Object under test
		ConfigurationService configService = ConfigurationService.getInstance();

		// Write a TestConfiguration to a temporary file.
		File testConfigurationFile = tmpFolder.newFile("java.d.conf");
		Files.write(testConfigurationFile.toPath(), "{ }".getBytes());

		// Test
		PluginDaemonConfiguration testConfig = configService.readConfiguration(testConfigurationFile,
				PluginDaemonConfiguration.class);

		// Verify
		assertEquals(PluginDaemonConfiguration.class, testConfig.getClass());
	}
}
