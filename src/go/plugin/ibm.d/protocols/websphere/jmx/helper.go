package jmx

import _ "embed"

//go:embed websphere_jmx_helper.jar
var helperJar []byte

const helperJarName = "websphere_jmx_helper.jar"
