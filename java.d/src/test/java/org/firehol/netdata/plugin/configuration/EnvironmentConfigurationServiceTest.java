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
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.nio.file.Path;
import java.nio.file.Paths;

import org.firehol.netdata.plugin.configuration.exception.EnvironmentConfigurationException;
import org.junit.Before;
import org.junit.Rule;
import org.junit.Test;
import org.junit.contrib.java.lang.system.EnvironmentVariables;

public class EnvironmentConfigurationServiceTest {

	@Rule
	public final EnvironmentVariables environmentVariables = new EnvironmentVariables();

	@Before
	public void setUp() throws EnvironmentConfigurationException {
		// reset environment variables to safe defaults & reload the service
		environmentVariables.set("NETDATA_CONFIG_DIR", "foo");
		environmentVariables.set("NETDATA_DEBUG_FLAGS", null);
		EnvironmentConfigurationService.reload();
	}

	@Test
	public void testReadNetdataConfigDir() throws EnvironmentConfigurationException {
		Path path = Paths.get("/test/folder");
		environmentVariables.set("NETDATA_CONFIG_DIR", path.toString());

		EnvironmentConfigurationService.reload();
		Path result = EnvironmentConfigurationService.getInstance().readNetdataConfigDir();

		assertEquals(path, result);
	}

	@Test
	public void testNetdataDebugDisabled() throws EnvironmentConfigurationException {
		environmentVariables.set("NETDATA_DEBUG_FLAGS", "0x0000000");
		EnvironmentConfigurationService.reload();
		assertFalse(EnvironmentConfigurationService.getInstance()
				.isDebugFlagSet(EnvironmentConfigurationService.D_PLUGINSD));
	}

	@Test
	public void testNetdataDebugEnabledHexadecimal() throws EnvironmentConfigurationException {
		environmentVariables.set("NETDATA_DEBUG_FLAGS", "0xFFFFFFF");
		EnvironmentConfigurationService.reload();
		assertTrue(EnvironmentConfigurationService.getInstance()
				.isDebugFlagSet(EnvironmentConfigurationService.D_PLUGINSD));
	}

	@Test
	public void testNetdataDebugEnabledDecimal() throws EnvironmentConfigurationException {
		environmentVariables.set("NETDATA_DEBUG_FLAGS", String.valueOf(2048));
		EnvironmentConfigurationService.reload();
		assertTrue(EnvironmentConfigurationService.getInstance()
				.isDebugFlagSet(EnvironmentConfigurationService.D_PLUGINSD));
	}

	@Test(expected = EnvironmentConfigurationException.class)
	public void testNetdataDebugInvalid() throws EnvironmentConfigurationException {
		environmentVariables.set("NETDATA_DEBUG_FLAGS", "Not-a-Number");
		EnvironmentConfigurationService.reload();
	}

}
