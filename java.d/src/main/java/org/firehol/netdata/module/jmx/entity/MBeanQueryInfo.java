package org.firehol.netdata.module.jmx.entity;

import javax.management.ObjectName;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class MBeanQueryInfo {

	private ObjectName mBeanName;

	private String mBeanAttribute;

	private Class<?> mBeanAttributeType;
}
