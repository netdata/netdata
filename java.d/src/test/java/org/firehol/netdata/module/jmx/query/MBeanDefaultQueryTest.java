package org.firehol.netdata.module.jmx.query;

import static org.junit.Assert.assertEquals;

import org.junit.Ignore;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.mockito.InjectMocks;
import org.mockito.junit.MockitoJUnitRunner;

@RunWith(MockitoJUnitRunner.class)
public class MBeanDefaultQueryTest {

	@InjectMocks
	public MBeanDefaultQuery mBeanQuery;

	@SuppressWarnings("PMD")
	@Ignore
	@Test
	public void testQuery() throws Exception {
		// TODO: would need to mock MBeanDefaultQuery.mBeanServer
		mBeanQuery.query();
	}

	@Test
	public void testToLongLong() {
		// Static Object
		long value = 1234;

		// Test
		long result = mBeanQuery.toLong(value);

		// Verify
		assertEquals(value, result);
	}

	@Test
	public void testToLongInteger() {
		// Static Object
		int value = 1234;

		// Test
		long result = mBeanQuery.toLong(value);

		// Verify
		assertEquals(1234, result);
	}

}
