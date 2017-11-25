package org.firehol.netdata.module.jmx.utils;

import java.io.IOException;

import javax.management.AttributeNotFoundException;
import javax.management.InstanceNotFoundException;
import javax.management.MBeanException;
import javax.management.MBeanServerConnection;
import javax.management.ObjectName;
import javax.management.ReflectionException;

import org.firehol.netdata.module.jmx.exception.JmxMBeanServerQueryException;

public final class MBeanServerUtils {

	private MBeanServerUtils() {
	}

	public static Object getAttribute(MBeanServerConnection mBeanServer, ObjectName mBeanName, String mBeanAttribute)
			throws JmxMBeanServerQueryException {
		try {
			return mBeanServer.getAttribute(mBeanName, mBeanAttribute);
		} catch (AttributeNotFoundException | InstanceNotFoundException | MBeanException | ReflectionException
				| IOException e) {
			throw new JmxMBeanServerQueryException(
					"Could not query attribute '" + mBeanAttribute + "' of MBean '" + mBeanName + "'", e);
		}

	}

}
