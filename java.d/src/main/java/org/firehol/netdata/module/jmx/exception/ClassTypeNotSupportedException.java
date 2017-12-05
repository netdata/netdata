package org.firehol.netdata.module.jmx.exception;

public class ClassTypeNotSupportedException extends JmxModuleException {
	private static final long serialVersionUID = 4131738122150987567L;

	public ClassTypeNotSupportedException() {
	}

	public ClassTypeNotSupportedException(String message) {
		super(message);
	}
}
