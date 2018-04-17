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

package org.firehol.netdata.module.jmx.utils;

import java.io.IOException;
import java.util.logging.Logger;

import javax.management.remote.JMXServiceURL;

import org.firehol.netdata.utils.LoggingUtils;

import com.sun.tools.attach.VirtualMachine;

import lombok.experimental.UtilityClass;

/**
 * Common methods for operation on a {@link VirtualMachine}.
 * 
 * There are no instances of this class.
 * 
 * @since 1.0.0
 * @author Simon Nagl
 */
@UtilityClass
public final class VirtualMachineUtils {

	private final Logger log = Logger.getLogger("org.firehol.netdata.module.jmx");

	private static final String SERVICE_URL_AGENT_PROPERTY_KEY = "com.sun.management.jmxremote.localConnectorAddress";

	/**
	 * Get the JMX ServiceUrl of a virtualMachine.
	 * 
	 * If {@code startJMXAgent} is true, try to start the jmxAgent if it is not
	 * started.
	 * 
	 * @param virtualMachine
	 *            Attached virtual machine to query.
	 * @param startJMXAgent
	 *            If true, try to start the jmxAgent if it is not started.
	 * @return a JMX ServiceUrl which can be used to connect to the
	 *         VirtualMachine or null if it could not be found.
	 * @throws IOException
	 *             Thrown when an IOException occurs while communicating with
	 *             the virtualMachine.
	 */
	public static JMXServiceURL getJMXServiceURL(VirtualMachine virtualMachine, boolean startJMXAgent)
			throws IOException {
		JMXServiceURL serviceUrl = getJMXServiceURL(virtualMachine);

		if (!startJMXAgent) {
			return serviceUrl;
		}

		if (serviceUrl != null) {
			return serviceUrl;
		}

		virtualMachine.startLocalManagementAgent();
		return getJMXServiceURL(virtualMachine);
	}

	/**
	 * Get the JMX ServiceUrl of a virtualMachine.
	 * 
	 * @param virtualMachine
	 *            Attached virtual machine to query.
	 * @return a JMX ServiceUrl which can be used to connect to the
	 *         VirtualMachine or null if it could not be found.
	 * @throws IOException
	 *             Thrown when an IOException occurs while communicating with
	 *             the virtualMachine.
	 */
	public static JMXServiceURL getJMXServiceURL(VirtualMachine virtualMachine) throws IOException {
		String serviceUrl = virtualMachine.getAgentProperties().getProperty(SERVICE_URL_AGENT_PROPERTY_KEY);

		if (serviceUrl == null) {
			return null;
		}

		try {
			return new JMXServiceURL(serviceUrl);
		} catch (Exception e) {
			log.warning(LoggingUtils.getMessageSupplier("Could not instantiate JMXServiceURL", e));
			return null;
		}
	}
}
