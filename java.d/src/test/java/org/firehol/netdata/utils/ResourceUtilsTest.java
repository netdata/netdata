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

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.io.Closeable;
import java.io.IOException;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;

import org.junit.Test;

public class ResourceUtilsTest {

	@Test
	public void testClose() throws InterruptedException, ExecutionException {

		// Static Objects
		Closeable resource = new Closeable() {
			@Override
			public void close() throws IOException {
				// Close with success.
			}
		};

		// Test
		CompletableFuture<Boolean> result = ResourceUtils.close(resource);

		// Verify
		assertTrue(result.get());
	}

	@Test
	public void testCloseFailure() throws InterruptedException, ExecutionException {

		// Static Objects
		Closeable resource = new Closeable() {
			@Override
			public void close() throws IOException {
				throw new IOException("Can not close resource");
			}
		};

		// Test
		CompletableFuture<Boolean> result = ResourceUtils.close(resource);

		// Verify
		assertFalse(result.get());
	}

}
