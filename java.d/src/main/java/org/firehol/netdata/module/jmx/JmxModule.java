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

package org.firehol.netdata.module.jmx;

import java.io.IOException;
import java.lang.management.ManagementFactory;
import java.util.ArrayList;
import java.util.Collection;
import java.util.HashSet;
import java.util.Iterator;
import java.util.LinkedList;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.function.Function;
import java.util.logging.Logger;
import java.util.stream.Collectors;

import javax.management.MBeanServerConnection;
import javax.management.remote.JMXConnector;
import javax.management.remote.JMXConnectorFactory;
import javax.management.remote.JMXServiceURL;

import org.firehol.netdata.exception.InitializationException;
import org.firehol.netdata.model.Chart;
import org.firehol.netdata.module.Module;
import org.firehol.netdata.module.jmx.configuration.JmxChartConfiguration;
import org.firehol.netdata.module.jmx.configuration.JmxModuleConfiguration;
import org.firehol.netdata.module.jmx.configuration.JmxServerConfiguration;
import org.firehol.netdata.module.jmx.exception.JmxMBeanServerConnectionException;
import org.firehol.netdata.module.jmx.exception.JmxMBeanServerQueryException;
import org.firehol.netdata.module.jmx.exception.VirtualMachineConnectionException;
import org.firehol.netdata.module.jmx.utils.VirtualMachineUtils;
import org.firehol.netdata.plugin.configuration.ConfigurationService;
import org.firehol.netdata.plugin.configuration.exception.ConfigurationSchemeInstantiationException;
import org.firehol.netdata.utils.ResourceUtils;
import org.firehol.netdata.utils.logging.LoggingUtils;
import org.firehol.netdata.utils.logging.NetdataLevel;

import com.sun.tools.attach.AttachNotSupportedException;
import com.sun.tools.attach.VirtualMachine;
import com.sun.tools.attach.VirtualMachineDescriptor;

import lombok.Getter;

/**
 * Netdata Java Plugin which collects metrics from JMX Servers.
 * 
 * <p>
 * The plugin also attatches to the local JMX server to monitor the plugin
 * daemon.
 * </p>
 * 
 * @see <a href=
 *      "http://www.oracle.com/technetwork/java/javase/tech/javamanagement-140525.html">Java
 *      Management Extensions (JMX) Technology</a>
 *
 */
public class JmxModule implements Module {

	private final Logger log = Logger.getLogger("org.firehol.netdata.module.jmx");

	private final ConfigurationService configurationService;

	@Getter
	private final String name;

	private JmxModuleConfiguration configuration;

	private final List<MBeanServerCollector> allMBeanCollector = new ArrayList<>();

	public JmxModule(ConfigurationService configurationService, String name) {
		this.configurationService = configurationService;
		this.name = name;
	}

	@Override
	public Collection<Chart> initialize() throws InitializationException {
		initConfiguration();
		connectToServers();

		List<Chart> charts = initCharts();
		return charts;
	}

	private void connectToServers() {
		connectToConfiguredServers();
		connectToLocalProcess();
		if (configuration.isAutoDetectLocalVirtualMachines()) {
			connectToLocalServers();
		}
	}

	private void connectToConfiguredServers() {
		for (JmxServerConfiguration serverConfiguartion : configuration.getJmxServers()) {
			MBeanServerCollector collector;
			try {
				collector = buildMBeanServerCollector(serverConfiguartion);
			} catch (JmxMBeanServerConnectionException e) {
				log.warning(LoggingUtils.buildMessage(e));
				continue;
			}

			allMBeanCollector.add(collector);
		}
	}

	private void connectToLocalProcess() {
		JmxServerConfiguration localConfiguration = new JmxServerConfiguration();
		localConfiguration.setCharts(configuration.getCommonCharts());
		localConfiguration.setName("JavaPluginDaemon");

		MBeanServerCollector collector = new MBeanServerCollector(localConfiguration,
				ManagementFactory.getPlatformMBeanServer(), getName());
		allMBeanCollector.add(collector);
	}

	private void connectToLocalServers() {
		Set<String> allRuntimeName = getAllMBeanCollectorRuntimeName();

		// List running VirtualMachines
		List<VirtualMachineDescriptor> virtualMachineDescriptors = VirtualMachine.list();
		for (VirtualMachineDescriptor virtualMachineDescriptor : virtualMachineDescriptors) {
			MBeanServerCollector collector;
			// Build the MBeanServerCollector
			try {
				collector = buildMBeanServerCollector(virtualMachineDescriptor);
			} catch (Exception e) {
				log.log(NetdataLevel.ERROR, LoggingUtils.getMessageSupplier(
						"Could not connect to JMX agent of process with PID " + virtualMachineDescriptor.id(), e));
				continue;
			}

			// Check if we already have a connection to this server...
			try {
				String runtimeName = collector.getRuntimeName();
				if (allRuntimeName.contains(runtimeName)) {
					// ... and close the connection if true.
					try {
						collector.close();
					} catch (IOException e) {
						log.warning(LoggingUtils.getMessageSupplier(
								"Could not close second connection to first configured and second auto detected JVM.",
								e));
					}
					continue;
				}
			} catch (JmxMBeanServerQueryException e) {
				log.warning(LoggingUtils.getMessageSupplier("Could not find runtimeName", e));
			}

			allMBeanCollector.add(collector);
		}
	}

	private Set<String> getAllMBeanCollectorRuntimeName() {
		Set<String> allRuntimeName = new HashSet<>();
		for (MBeanServerCollector mBeanCollector : allMBeanCollector) {
			try {
				String runtimeName = mBeanCollector.getRuntimeName();
				allRuntimeName.add(runtimeName);
			} catch (JmxMBeanServerQueryException e) {
				log.warning(LoggingUtils.getMessageSupplier("Could not find runtimeName", e));
			}
		}
		return allRuntimeName;
	}

	private void initConfiguration() throws InitializationException {
		readConfiguration();
		propagateCommonChartsToServerConfiguration();
	}

	private void readConfiguration() throws InitializationException {
		try {
			configuration = configurationService.readModuleConfiguration(getName(), JmxModuleConfiguration.class);
		} catch (ConfigurationSchemeInstantiationException e) {
			throw new InitializationException("Could not read jmx plugin configuration", e);
		}
	}

	private void propagateCommonChartsToServerConfiguration() {
		for (JmxServerConfiguration serverConfiguartion : configuration.getJmxServers()) {
			if (serverConfiguartion.getCharts() == null) {
				serverConfiguartion.setCharts(configuration.getCommonCharts());
				continue;
			}

			Map<String, JmxChartConfiguration> chartConfigById = chartConfigurationsById(
					serverConfiguartion.getCharts());

			for (JmxChartConfiguration chartConfig : configuration.getCommonCharts()) {
				chartConfigById.putIfAbsent(chartConfig.getId(), chartConfig);
			}

			List<JmxChartConfiguration> chartConfigs = chartConfigById.values().stream().collect(Collectors.toList());
			serverConfiguartion.setCharts(chartConfigs);
		}
	}

	private List<Chart> initCharts() {
		List<Chart> allChart = new LinkedList<>();
		Iterator<MBeanServerCollector> mBeanCollectorIterator = allMBeanCollector.iterator();

		while (mBeanCollectorIterator.hasNext()) {
			MBeanServerCollector mBeanCollector = mBeanCollectorIterator.next();
			try {
				allChart.addAll(mBeanCollector.initialize());
			} catch (InitializationException e) {
				log.log(NetdataLevel.ERROR, LoggingUtils.buildMessage(
						"Could not initialize JMX plugin " + mBeanCollector.getMBeanServer().toString(), e));
				ResourceUtils.close(mBeanCollector);
				mBeanCollectorIterator.remove();
			}
		}

		return allChart;
	}

	private Map<String, JmxChartConfiguration> chartConfigurationsById(List<JmxChartConfiguration> charts) {
		return charts.stream().collect(Collectors.toMap(JmxChartConfiguration::getId, Function.identity()));
	}

	protected MBeanServerCollector buildMBeanServerCollector(JmxServerConfiguration config)
			throws JmxMBeanServerConnectionException {

		JMXConnector connection = null;

		try {
			JMXServiceURL url = new JMXServiceURL(config.getServiceUrl());
			connection = JMXConnectorFactory.connect(url);
			MBeanServerConnection server = connection.getMBeanServerConnection();
			MBeanServerCollector collector = new MBeanServerCollector(config, server, connection, getName());
			return collector;
		} catch (IOException e) {
			if (connection != null) {
				ResourceUtils.close(connection);
			}
			throw new JmxMBeanServerConnectionException(
					"Faild to connect to JMX Server " + config.getServiceUrl() + ".", e);
		}
	}

	protected MBeanServerCollector buildMBeanServerCollector(VirtualMachineDescriptor virtualMachineDescriptor)
			throws VirtualMachineConnectionException, JmxMBeanServerConnectionException {
		VirtualMachine virtualMachine = null;

		try {
			virtualMachine = VirtualMachine.attach(virtualMachineDescriptor);

			JMXServiceURL serviceUrl;
			try {
				serviceUrl = VirtualMachineUtils.getJMXServiceURL(virtualMachine, true);
			} catch (IOException e) {
				throw new VirtualMachineConnectionException(
						"Could not get JMX ServiceUrl from Virtual Machine with PID " + virtualMachine.id(), e);

			}

			// Build configuration
			JmxServerConfiguration config = new JmxServerConfiguration();
			config.setServiceUrl(serviceUrl.toString());

			config.setName(virtualMachine.id());
			if (configuration != null) {
				config.setCharts(configuration.getCommonCharts());
			}

			// Build the MBeanServerCollector
			return buildMBeanServerCollector(config);

		} catch (AttachNotSupportedException | IOException e) {
			throw new VirtualMachineConnectionException(
					"Could not attatch to virtualMachine with PID " + virtualMachineDescriptor.id(), e);

		} finally {
			// Detatch from virtual machine.
			try {
				if (virtualMachine != null) {
					virtualMachine.detach();
				}
			} catch (IOException e) {
				log.warning(LoggingUtils.getMessageSupplier(
						"Could not detatch from virtual machine with PID " + virtualMachine.id(), e));
			}
		}
	}

	public void cleanup() {
		try {
			CompletableFuture
					.allOf(allMBeanCollector.stream().map(ResourceUtils::close).toArray(CompletableFuture[]::new))
					.get();
		} catch (InterruptedException | ExecutionException e) {
			log.fine("Could not close connection to at least one JMX Server");
		}

	}

	@Override
	public Collection<Chart> collectValues() {
		return allMBeanCollector.stream().map(MBeanServerCollector::collectValues).flatMap(Collection::stream).collect(
				Collectors.toList());
	}

	public static final class Builder implements Module.Builder {

		@Override
		public Module build(ConfigurationService configurationService, String moduleName) {
			return new JmxModule(configurationService, moduleName);
		}

	}
}
