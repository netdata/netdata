package org.firehol.netdata.module.jmx.entity;

import javax.management.MBeanServerConnection;
import javax.management.ObjectName;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class MBeanQueryInfo {

	private MBeanServerConnection mBeanServer;

	private ObjectName mBeanName;

	private String mBeanAttribute;

	private Object mBeanAttributeExample;
}
