package org.firehol.netdata.module.jmx;

import static org.junit.Assert.assertEquals;

import org.junit.Test;
import org.junit.runner.RunWith;
import org.mockito.InjectMocks;
import org.mockito.junit.MockitoJUnitRunner;

@RunWith(MockitoJUnitRunner.class)
public class MBeanDefaultQueryTest {

	@InjectMocks
	public MBeanDefaultQuery mBeanQuery;

	@Test
	public void testQuery() {
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

	@Test
	public void testToLongDouble() {
		// Static Object
		double value = 1234;

		// Test
		long result = mBeanQuery.toLong(value);

		// Verify
		assertEquals(1234 * 100, result);
	}

}
