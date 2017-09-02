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

package org.firehol.netdata.utils;

import static org.junit.Assert.assertEquals;

import java.util.function.Supplier;

import org.junit.Before;
import org.junit.Test;

public class LoggingUtilsTest {

	private Exception exception;

	@Before
	public void init() {
		Exception fine = new Exception("Here are the details.");
		Exception detail = new Exception("This is the reason.", fine);
		exception = new Exception("Something went wrong.", detail);
	}

	@Test
	public void testBuildMessageThrowable() {
		// Test
		String message = LoggingUtils.buildMessage(exception);

		// Verify
		assertEquals("Something went wrong. Detail: This is the reason. Detail: Here are the details.", message);
	}

	@Test
	public void testBuildMessageStringThrowable() {
		// Test
		String message = LoggingUtils.buildMessage("Could not do it.", exception);

		// Verify
		assertEquals(
				"Could not do it. Reason: Something went wrong. Detail: This is the reason. Detail: Here are the details.",
				message);
	}

	@Test
	public void testBuildMessageStrings() {
		// Test
		String message = LoggingUtils.buildMessage("This ", "should ", "be ", "one ", "message.");

		// Verify
		assertEquals("This should be one message.", message);
	}

	@Test
	public void testBuildMessageStringsNoArg() {
		// Test
		String message = LoggingUtils.buildMessage();

		// Verify
		assertEquals("", message);
	}

	@Test
	public void testBuildMessageStringsOneArg() {
		// Test
		String message = LoggingUtils.buildMessage("One Argument.");

		// Verify
		assertEquals("One Argument.", message);
	}

	@Test
	public void getMessageSupplierThrowable() {
		// Test
		Supplier<String> messageSupplier = LoggingUtils.getMessageSupplier(exception);

		// Verify
		assertEquals("Something went wrong. Detail: This is the reason. Detail: Here are the details.",
				messageSupplier.get());
	}

	@Test
	public void getMessageSupplierStringThrowable() {
		// Test
		Supplier<String> messageSupplier = LoggingUtils.getMessageSupplier("Could not do it.", exception);

		// Verify
		assertEquals(
				"Could not do it. Reason: Something went wrong. Detail: This is the reason. Detail: Here are the details.",
				messageSupplier.get());

	}

	public void getMessageSuplierStrings() {
		// Test
		Supplier<String> messageSupplier = LoggingUtils.getMessageSupplier("This ", "should ", "be ", "one ",
				"message.");

		// Verify
		assertEquals("This should be one message.", messageSupplier.get());

	}

}
