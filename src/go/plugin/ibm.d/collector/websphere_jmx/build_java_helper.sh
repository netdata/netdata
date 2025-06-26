#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

# Change to the directory containing this script
cd "$(dirname "$0")"

echo "Building WebSphere JMX helper..."

# Compile the Java file
javac -cp "." websphere_jmx_helper.java

# Create the JAR file
jar cf websphere_jmx_helper.jar websphere_jmx_helper.class

# Clean up the class file
rm -f websphere_jmx_helper.class

echo "Successfully built websphere_jmx_helper.jar"