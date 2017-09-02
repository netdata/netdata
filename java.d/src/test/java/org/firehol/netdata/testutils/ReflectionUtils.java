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

package org.firehol.netdata.testutils;

import java.lang.reflect.Field;

public abstract class ReflectionUtils {
	/**
	 * Getter for private field {@code filedName} of {@code object}.
	 *
	 * @param object
	 *            to access
	 * @param fieldName
	 *            Name of field to read.
	 * @return the value of the field.
	 * @throws NoSuchFieldException
	 *             if a field with the specified name is not found
	 * @throws IllegalAccessException
	 *             if this Field object is enforcing Java language access control
	 *             and the underlying field is inaccessible.
	 * @throws SecurityException
	 *             If a security manager, <i>s</i>, is present and any of the
	 *             following conditions is met:
	 *             <ul>
	 *             <li>the caller's class loader is not the same as the class loader
	 *             of this class and invocation of
	 *             {@link SecurityManager#checkPermission s.checkPermission} method
	 *             with {@code RuntimePermission("accessDeclaredMembers")} denies
	 *             access to the declared field
	 *             <li>the caller's class loader is not the same as or an ancestor
	 *             of the class loader for the current class and invocation of
	 *             {@link SecurityManager#checkPackageAccess s.checkPackageAccess()}
	 *             denies access to the package of this class
	 *             <li>if <i>s</i> denies making the field accessible.
	 *             </ul>
	 */
	public static Object getPrivateField(Object object, String fieldName)
			throws NoSuchFieldException, IllegalAccessException, SecurityException {
		Field field = object.getClass().getDeclaredField(fieldName);
		field.setAccessible(true);
		return field.get(object);
	}

	/**
	 * Setter for private filed {@code filedName} of {@code object}.
	 *
	 * @param object
	 *            to modify
	 * @param fieldName
	 *            Name of field to set.
	 * @param value
	 *            to set
	 * @throws NoSuchFieldException
	 *             if a field with the specified name is not found.
	 * @throws NullPointerException
	 *             If any of the following conditions is met:
	 *             <ul>
	 *             <li>if {@code filedName} is null</li>
	 *             <li>if the {@code object} is {@code null} and the field is an
	 *             instance field.</li>
	 *             </ul>
	 * @throws SecurityException
	 *             If a security manager, <i>s</i>, is present and any of the
	 *             following conditions is met:
	 *
	 *             <ul>
	 *             <li>the caller's class loader is not the same as the class loader
	 *             of this class and invocation of
	 *             {@link SecurityManager#checkPermission s.checkPermission} method
	 *             with {@code RuntimePermission("accessDeclaredMembers")} denies
	 *             access to the declared field
	 *             <li>the caller's class loader is not the same as or an ancestor
	 *             of the class loader for the current class and invocation of
	 *             {@link SecurityManager#checkPackageAccess s.checkPackageAccess()}
	 *             denies access to the package of this class
	 *             <li>if <i>s</i> denies making the field accessible.
	 *             </ul>
	 * @throws IllegalArgumentException
	 *             if the specified object is not an instance of the class or
	 *             interface declaring the underlying field (or a subclass or
	 *             implementor thereof), or if an unwrapping conversion fails.
	 * @throws IllegalAccessException
	 *             if this Field object is enforcing Java language access control
	 *             and the underlying field is either inaccessible or final.
	 * @throws ExceptionInInitializerError
	 *             if the initialization provoked by this method fails.
	 */
	public static void setPrivateFiled(Object object, String fieldName, Object value)
			throws NoSuchFieldException, SecurityException, IllegalArgumentException, IllegalAccessException,
			NullPointerException, ExceptionInInitializerError {
		Field field = object.getClass().getDeclaredField(fieldName);
		field.setAccessible(true);
		field.set(object, value);
	}
}
