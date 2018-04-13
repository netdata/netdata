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

import java.io.PrintWriter;
import java.io.StringWriter;
import java.util.function.Supplier;

public abstract class LoggingUtils {

	private static void appendMessage(Throwable reason, StringBuilder sb) {

		sb.append(reason.getClass().getSimpleName());
		sb.append(": ");
		sb.append(reason.getMessage());

		Throwable detail = reason.getCause();
		while (detail != null) {
			sb.append(" Detail: ");
			sb.append(detail.getMessage());
			detail = detail.getCause();
		}
	}

	public static String buildMessage(Throwable reason) {
		StringBuilder sb = new StringBuilder();
		appendMessage(reason, sb);
		return sb.toString();
	}

	public static String buildTrace(String message, Throwable reason) {
		StringBuilder sb = new StringBuilder(message);

		sb.append(" Reason: ");
		appendMessage(reason, sb);
		sb.append(" Trace:\n");
		StringWriter sw = new StringWriter();
		reason.printStackTrace(new PrintWriter(sw));
		sb.append(sw.toString());
		return sb.toString();
	}

	public static String buildMessage(String message, Throwable reason) {
		StringBuilder sb = new StringBuilder(message);

		sb.append(" Reason: ");
		appendMessage(reason, sb);
		return sb.toString();
	}

	public static String buildMessage(String... messages) {
		if (messages.length == 0) {
			return "";
		}

		if (messages.length == 1) {
			return messages[0];
		}

		// Find String length.
		int totalLength = 0;
		for (String message : messages) {
			totalLength = +message.length();
		}

		StringBuilder sb = new StringBuilder(totalLength);

		// Build the message
		for (String message : messages) {
			sb.append(message);
		}

		return sb.toString();
	}

	public static Supplier<String> getMessageSupplier(Throwable reason) {
		return new Supplier<String>() {
			@Override
			public String get() {
				return buildMessage(reason);
			}
		};
	}

	public static Supplier<String> getMessageSupplier(String message, Throwable reason) {
		return new Supplier<String>() {
			@Override
			public String get() {
				return buildMessage(message, reason);
			}
		};
	}

	public static Supplier<String> getMessageSupplier(String... messages) {
		return new Supplier<String>() {
			@Override
			public String get() {
				return buildMessage(messages);
			}
		};
	}
}
