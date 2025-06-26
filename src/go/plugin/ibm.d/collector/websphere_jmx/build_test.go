// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_jmx

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJMXHelperJarEmbedded(t *testing.T) {
	// Verify that the JAR file is embedded
	assert.NotNil(t, jmxHelperJar)
	assert.Greater(t, len(jmxHelperJar), 0)
	
	// JAR files start with PK (ZIP format)
	if len(jmxHelperJar) >= 2 {
		assert.Equal(t, byte('P'), jmxHelperJar[0])
		assert.Equal(t, byte('K'), jmxHelperJar[1])
	}
}

func TestBuildInstructions(t *testing.T) {
	// Check that build script exists
	_, err := os.Stat("build_java_helper.sh")
	if os.IsNotExist(err) {
		t.Skip("build_java_helper.sh not found in test environment")
	}
	assert.NoError(t, err)
}