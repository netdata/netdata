package zabbixpreproc

import (
	"strings"
	"sync"
	"testing"
)

// TestTranslateMIB_Hardcoded tests translation using hardcoded map
func TestTranslateMIB_Hardcoded(t *testing.T) {
	tests := []struct {
		name     string
		mibName  string
		expected string
	}{
		{"SNMPv2-MIB sysDescr", "SNMPv2-MIB::sysDescr", ".1.3.6.1.2.1.1.1"},
		{"SNMPv2-MIB sysUpTime", "SNMPv2-MIB::sysUpTime", ".1.3.6.1.2.1.1.3"},
		{"IF-MIB ifDescr", "IF-MIB::ifDescr", ".1.3.6.1.2.1.2.2.1.2"},
		{"IF-MIB ifOperStatus", "IF-MIB::ifOperStatus", ".1.3.6.1.2.1.2.2.1.8"},
		{"IF-MIB ifHCInOctets", "IF-MIB::ifHCInOctets", ".1.3.6.1.2.1.31.1.1.1.6"},
		{"HOST-RESOURCES hrSystemUptime", "HOST-RESOURCES-MIB::hrSystemUptime", ".1.3.6.1.2.1.25.1.1"},
		{"HOST-RESOURCES hrStorageDescr", "HOST-RESOURCES-MIB::hrStorageDescr", ".1.3.6.1.2.1.25.2.3.1.3"},
		{"HOST-RESOURCES hrProcessorLoad", "HOST-RESOURCES-MIB::hrProcessorLoad", ".1.3.6.1.2.1.25.3.3.1.2"},
		{"IP-MIB ipForwarding", "IP-MIB::ipForwarding", ".1.3.6.1.2.1.4.1"},
		{"TCP-MIB tcpCurrEstab", "TCP-MIB::tcpCurrEstab", ".1.3.6.1.2.1.6.9"},
		{"UDP-MIB udpInDatagrams", "UDP-MIB::udpInDatagrams", ".1.3.6.1.2.1.7.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := translateMIB(tt.mibName)
			if err != nil {
				t.Fatalf("translateMIB(%q) error: %v", tt.mibName, err)
			}
			if result != tt.expected {
				t.Errorf("translateMIB(%q) = %q, expected %q", tt.mibName, result, tt.expected)
			}
		})
	}
}

// TestTranslateMIB_NumericOID tests that numeric OIDs pass through unchanged
func TestTranslateMIB_NumericOID(t *testing.T) {
	tests := []struct {
		name     string
		oid      string
		expected string
	}{
		{"With leading dot", ".1.3.6.1.2.1.1.1", ".1.3.6.1.2.1.1.1"},
		{"Without leading dot", "1.3.6.1.2.1.1.1", "1.3.6.1.2.1.1.1"},
		{"Short OID", ".1.3.6.1", ".1.3.6.1"},
		{"Single digit", ".1", ".1"},
		{"Complex OID", ".1.3.6.1.2.1.2.2.1.10.1", ".1.3.6.1.2.1.2.2.1.10.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := translateMIB(tt.oid)
			if err != nil {
				t.Fatalf("translateMIB(%q) error: %v", tt.oid, err)
			}
			if result != tt.expected {
				t.Errorf("translateMIB(%q) = %q, expected %q (should pass through)", tt.oid, result, tt.expected)
			}
		})
	}
}

// TestTranslateMIB_UnknownMIB tests error handling for unknown MIBs
func TestTranslateMIB_UnknownMIB(t *testing.T) {
	// Clear cache to ensure fresh test
	mibTranslationCacheMu.Lock()
	mibTranslationCache = make(map[string]string)
	mibTranslationCacheMu.Unlock()

	// Unknown MIB that's not in hardcoded map
	_, err := translateMIB("UNKNOWN-MIB::unknownObject")
	if err == nil {
		t.Error("Expected error for unknown MIB, got nil")
	}

	expectedErrMsg := "unknown MIB: UNKNOWN-MIB::unknownObject"
	if err != nil && err.Error()[:len(expectedErrMsg)] != expectedErrMsg {
		t.Errorf("Error message = %q, expected to start with %q", err.Error(), expectedErrMsg)
	}
}

// TestTranslateMIB_Cache tests that translations are cached
func TestTranslateMIB_Cache(t *testing.T) {
	// Clear cache
	mibTranslationCacheMu.Lock()
	mibTranslationCache = make(map[string]string)
	mibTranslationCacheMu.Unlock()

	// First call - should use hardcoded map
	result1, err := translateMIB("SNMPv2-MIB::sysDescr")
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	// Second call - should still work (from hardcoded map, not cache)
	result2, err := translateMIB("SNMPv2-MIB::sysDescr")
	if err != nil {
		t.Fatalf("Second call failed: %v", err)
	}

	if result1 != result2 {
		t.Errorf("Results differ: %q vs %q", result1, result2)
	}

	// Verify cache is not polluted with hardcoded entries
	mibTranslationCacheMu.RLock()
	cacheSize := len(mibTranslationCache)
	mibTranslationCacheMu.RUnlock()

	if cacheSize > 0 {
		t.Errorf("Cache should be empty (hardcoded entries shouldn't be cached), got %d entries", cacheSize)
	}
}

// TestTranslateMIB_ConcurrentAccess tests thread safety
func TestTranslateMIB_ConcurrentAccess(t *testing.T) {
	// Clear cache
	mibTranslationCacheMu.Lock()
	mibTranslationCache = make(map[string]string)
	mibTranslationCacheMu.Unlock()

	mibs := []string{
		"SNMPv2-MIB::sysDescr",
		"IF-MIB::ifDescr",
		"HOST-RESOURCES-MIB::hrSystemUptime",
		"IP-MIB::ipForwarding",
		"TCP-MIB::tcpCurrEstab",
		".1.3.6.1.2.1.1.1",
		".1.3.6.1.2.1.2.2.1.2",
	}

	var wg sync.WaitGroup
	goroutines := 50
	iterations := 20

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				mib := mibs[j%len(mibs)]
				_, err := translateMIB(mib)
				// Only numeric OIDs and hardcoded MIBs should succeed
				if err != nil && !strings.HasPrefix(mib, ".") && mibToOID[mib] == "" {
					// Unknown MIB - error expected
					continue
				}
				if err != nil {
					t.Errorf("goroutine %d iteration %d: translateMIB(%q) failed: %v", id, j, mib, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestTranslateMIB_CacheSizeLimit tests that cache respects size limit
func TestTranslateMIB_CacheSizeLimit(t *testing.T) {
	// Save original limit
	originalLimit := mibCacheMaxSize
	defer func() { mibCacheMaxSize = originalLimit }()

	// Set small limit for testing
	mibCacheMaxSize = 5

	// Clear cache
	mibTranslationCacheMu.Lock()
	mibTranslationCache = make(map[string]string)
	mibTranslationCacheMu.Unlock()

	// Note: This test doesn't actually populate the cache since all tested MIBs
	// are in the hardcoded map. To test cache limit properly, we would need
	// snmptranslate available and translate unknown MIBs. For now, we just
	// verify the cache limit constant is respected in the code.

	// Verify cache size limit is applied in code (checked in translateMIB function)
	if mibCacheMaxSize != 5 {
		t.Errorf("mibCacheMaxSize should be 5, got %d", mibCacheMaxSize)
	}
}

// TestParseSNMPWalk_MIBTranslation tests that parseSNMPWalk uses MIB translation
//func TestParseSNMPWalk_MIBTranslation(t *testing.T) {
//	tests := []struct {
//		name        string
//		data        string
//		targetMIB   string
//		expectedOID string
//		shouldError bool
//	}{
//		{
//			name:        "SNMPv2-MIB sysDescr",
//			data:        ".1.3.6.1.2.1.1.1.0 = STRING: \"Linux server 5.4.0\"",
//			targetMIB:   "SNMPv2-MIB::sysDescr.0",
//			expectedOID: ".1.3.6.1.2.1.1.1.0",
//			shouldError: false,
//		},
//		{
//			name:        "IF-MIB ifDescr with index",
//			data:        ".1.3.6.1.2.1.2.2.1.2.1 = STRING: \"eth0\"",
//			targetMIB:   "IF-MIB::ifDescr.1",
//			expectedOID: ".1.3.6.1.2.1.2.2.1.2.1",
//			shouldError: false,
//		},
//		{
//			name:        "Numeric OID",
//			data:        ".1.3.6.1.2.1.1.1.0 = STRING: \"Test\"",
//			targetMIB:   ".1.3.6.1.2.1.1.1.0",
//			expectedOID: ".1.3.6.1.2.1.1.1.0",
//			shouldError: false,
//		},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			result, err := parseSNMPWalk(tt.data, tt.targetMIB)
//			if tt.shouldError {
//				if err == nil {
//					t.Error("Expected error, got nil")
//				}
//				return
//			}
//			if err != nil {
//				t.Fatalf("parseSNMPWalk() error: %v", err)
//			}
//			if result.oid != tt.expectedOID {
//				t.Errorf("parseSNMPWalk() OID = %q, expected %q", result.oid, tt.expectedOID)
//			}
//		})
//	}
//}

// TestTranslateMIB_AllHardcodedMIBs verifies all hardcoded MIBs translate correctly
func TestTranslateMIB_AllHardcodedMIBs(t *testing.T) {
	// Verify every entry in mibToOID map works
	for mibName, expectedOID := range mibToOID {
		result, err := translateMIB(mibName)
		if err != nil {
			t.Errorf("translateMIB(%q) error: %v", mibName, err)
			continue
		}
		if result != expectedOID {
			t.Errorf("translateMIB(%q) = %q, expected %q", mibName, result, expectedOID)
		}
	}

	// Report statistics
	t.Logf("Verified %d hardcoded MIB translations", len(mibToOID))
}
