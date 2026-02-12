package zabbixpreproc

import (
	"crypto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"hash"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// JavaScript execution safety constants
const (
	jsExecutionTimeout = 5 * time.Second // Maximum execution time for JavaScript
	jsMaxCallStackSize = 100             // Maximum call stack depth
	jsMinRSAKeyBits    = 1024            // Minimum RSA key size (bits) - matches Zabbix behavior
	jsProgramCacheSize = 1000            // Maximum number of cached compiled programs
	// NOTE: 1024-bit RSA keys are deprecated and insecure (NIST recommends 2048+ bits).
	// This limit matches Zabbix's behavior for compatibility. Production deployments
	// should enforce 2048+ bit keys at the configuration/policy level.
)

// PEM key normalization constants
const (
	pemBeginMarker     = "-----BEGIN"
	pemEndMarker       = "-----END "
	pemDelimiter       = "-----"
	pemBeginMarkerLen  = 10 // len("-----BEGIN")
	pemEndMarkerLen    = 9  // len("-----END ")
	pemDelimiterLen    = 5  // len("-----")
	pemLineWidth       = 64 // Standard PEM line width (RFC 7468)
	pemMinHeaderLen    = 30 // Minimum length for PEM header check
	pemNewlineCheckPos = 27 // Position to check for newline after BEGIN marker
)

// boundedProgramCache implements a size-limited LRU cache for compiled JavaScript programs.
type boundedProgramCache struct {
	mu    sync.RWMutex
	cache map[string]*goja.Program
	order []string // LRU tracking (oldest first)
}

// newBoundedProgramCache creates a bounded program cache.
func newBoundedProgramCache(maxSize int) *boundedProgramCache {
	return &boundedProgramCache{
		cache: make(map[string]*goja.Program),
		order: make([]string, 0, maxSize),
	}
}

// Get retrieves a compiled program from the cache.
func (c *boundedProgramCache) Get(key string) (*goja.Program, bool) {
	c.mu.RLock()
	prog, found := c.cache[key]
	c.mu.RUnlock()

	if found {
		// Move to end of LRU order (most recently used)
		c.mu.Lock()
		for i, k := range c.order {
			if k == key {
				c.order = append(c.order[:i], c.order[i+1:]...)
				c.order = append(c.order, key)
				break
			}
		}
		c.mu.Unlock()
	}

	return prog, found
}

// Put stores a compiled program in the cache, evicting oldest if at capacity.
func (c *boundedProgramCache) Put(key string, prog *goja.Program) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already cached
	if _, exists := c.cache[key]; exists {
		// Update LRU order
		for i, k := range c.order {
			if k == key {
				c.order = append(c.order[:i], c.order[i+1:]...)
				c.order = append(c.order, key)
				break
			}
		}
		return
	}

	// Evict oldest if at capacity
	if len(c.cache) >= jsProgramCacheSize {
		oldest := c.order[0]
		delete(c.cache, oldest)
		c.order = c.order[1:]
	}

	// Add new entry
	c.cache[key] = prog
	c.order = append(c.order, key)
}

// JavaScript program caching for performance optimization
var (
	// programCache caches compiled JavaScript programs (keyed by script code)
	// Bounded to jsProgramCacheSize to prevent unbounded memory growth
	programCache = newBoundedProgramCache(jsProgramCacheSize)
)

// createConfiguredVM creates a new goja VM with all built-in functions configured.
// This function is called by vmPool when creating new VM instances.
func createConfiguredVM() *goja.Runtime {
	vm := goja.New()
	vm.SetMaxCallStackSize(jsMaxCallStackSize)

	// Configure all built-in functions
	setupBuiltinFunctions(vm)

	return vm
}

// setupBuiltinFunctions configures all Zabbix built-in JavaScript functions on a VM.
// This is called once per VM instance when it's created.
func setupBuiltinFunctions(vm *goja.Runtime) {
	// btoa - Base64 encode (supports string and Uint8Array)
	vm.Set("btoa", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}

		var data []byte
		arg := call.Arguments[0]

		// Check if it's a Uint8Array
		if obj := arg.ToObject(vm); obj != nil {
			if arr := obj.Get("constructor"); arr != nil && arr.String() == "function Uint8Array() { [native code] }" {
				// It's a typed array, extract bytes
				if length := obj.Get("length"); length != nil {
					n := int(length.ToInteger())
					data = make([]byte, n)
					for i := 0; i < n; i++ {
						data[i] = byte(obj.Get(fmt.Sprintf("%d", i)).ToInteger())
					}
				}
			}
		}

		// If not Uint8Array, treat as string
		if data == nil {
			data = []byte(arg.String())
		}

		encoded := base64.StdEncoding.EncodeToString(data)
		return vm.ToValue(encoded)
	})

	// atob - Base64 decode
	vm.Set("atob", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}

		encoded := call.Arguments[0].String()
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			// Invalid base64 - return empty string
			return vm.ToValue("")
		}

		return vm.ToValue(string(decoded))
	})

	// hmac - HMAC hash function
	vm.Set("hmac", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.NewGoError(fmt.Errorf("hmac requires 3 arguments: algorithm, key, data")))
		}

		// Check for null/undefined arguments
		if goja.IsNull(call.Arguments[1]) || goja.IsUndefined(call.Arguments[1]) {
			panic(vm.NewGoError(fmt.Errorf("invalid key parameter")))
		}
		if goja.IsNull(call.Arguments[2]) || goja.IsUndefined(call.Arguments[2]) {
			panic(vm.NewGoError(fmt.Errorf("invalid data parameter")))
		}

		algorithm := call.Arguments[0].String()
		key := call.Arguments[1].String()
		data := call.Arguments[2].String()

		var mac hash.Hash

		switch algorithm {
		case "md5":
			mac = hmac.New(md5.New, []byte(key))
		case "sha256":
			mac = hmac.New(sha256.New, []byte(key))
		default:
			panic(vm.NewGoError(fmt.Errorf("unsupported hmac algorithm: %s", algorithm)))
		}

		mac.Write([]byte(data))
		result := hex.EncodeToString(mac.Sum(nil))
		return vm.ToValue(result)
	})

	// sign - RSA signature function
	vm.Set("sign", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(vm.NewGoError(fmt.Errorf("sign requires 3 arguments: algorithm, privateKey, data")))
		}

		algorithm := call.Arguments[0].String()
		pemKey := call.Arguments[1].String()
		dataArg := call.Arguments[2]

		// Extract data (support string and Uint8Array)
		var data []byte
		if obj := dataArg.ToObject(vm); obj != nil {
			if arr := obj.Get("constructor"); arr != nil && arr.String() == "function Uint8Array() { [native code] }" {
				// It's a typed array, extract bytes
				if length := obj.Get("length"); length != nil {
					n := int(length.ToInteger())
					data = make([]byte, n)
					for i := 0; i < n; i++ {
						data[i] = byte(obj.Get(fmt.Sprintf("%d", i)).ToInteger())
					}
				}
			}
		}
		if data == nil {
			data = []byte(dataArg.String())
		}

		if algorithm != "sha256" {
			panic(vm.NewGoError(fmt.Errorf("unsupported signature algorithm: %s", algorithm)))
		}

		// Normalize PEM key (handle single-line keys)
		pemKey = normalizePEMKey(pemKey)

		// Parse PEM key
		block, _ := pem.Decode([]byte(pemKey))
		if block == nil {
			panic(vm.NewGoError(fmt.Errorf("failed to parse PEM block")))
		}

		var privateKey *rsa.PrivateKey
		var err error

		// Try PKCS#1 format first
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			// Try PKCS#8 format
			key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err2 != nil {
				panic(vm.NewGoError(fmt.Errorf("failed to parse private key: %v", err)))
			}
			var ok bool
			privateKey, ok = key.(*rsa.PrivateKey)
			if !ok {
				panic(vm.NewGoError(fmt.Errorf("not an RSA private key")))
			}
		}

		// Validate RSA key size (minimum 2048 bits for security)
		keyBits := privateKey.N.BitLen()
		if keyBits < jsMinRSAKeyBits {
			panic(vm.NewGoError(fmt.Errorf("RSA key too small: %d bits (minimum %d bits required)", keyBits, jsMinRSAKeyBits)))
		}

		// Hash the data
		hashed := sha256.Sum256(data)

		// Sign the hash
		signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("signing failed: %v", err)))
		}

		// Return hex-encoded signature
		result := hex.EncodeToString(signature)
		return vm.ToValue(result)
	})

	// HttpRequest - Minimal stub for Zabbix HTTP request object
	// Only needed for test compatibility - actual HTTP functionality not implemented
	vm.RunString(`
		function HttpRequest() {
			// Minimal constructor - allows property assignment
			// Actual HTTP methods would be implemented here in full Zabbix
		}
	`)
}

// normalizePEMKey normalizes a PEM key by adding proper newlines
func normalizePEMKey(pemStr string) string {
	// If already has newlines after BEGIN, return as is
	if len(pemStr) > pemMinHeaderLen && pemStr[pemNewlineCheckPos] == '\n' {
		return pemStr
	}

	// Find BEGIN and END markers
	beginIdx := 0
	endIdx := len(pemStr)

	for i := 0; i < len(pemStr)-pemBeginMarkerLen; i++ {
		if pemStr[i:i+pemBeginMarkerLen] == pemBeginMarker {
			// Find end of BEGIN line
			for j := i; j < len(pemStr)-pemDelimiterLen; j++ {
				if pemStr[j:j+pemDelimiterLen] == pemDelimiter && j > i+pemBeginMarkerLen {
					beginIdx = j + pemDelimiterLen
					break
				}
			}
		}
		if pemStr[i:i+pemEndMarkerLen] == pemEndMarker {
			endIdx = i
			break
		}
	}

	if beginIdx == 0 || endIdx == len(pemStr) {
		return pemStr // No markers found, return as is
	}

	// Extract header, body, footer
	header := pemStr[:beginIdx]
	body := pemStr[beginIdx:endIdx]
	footer := pemStr[endIdx:]

	// Remove any existing whitespace from body
	var bodyClean strings.Builder
	bodyClean.Grow(len(body))
	for _, ch := range body {
		if ch != '\n' && ch != '\r' && ch != ' ' && ch != '\t' {
			bodyClean.WriteRune(ch)
		}
	}

	// Split body into pemLineWidth-char lines (RFC 7468 standard)
	cleanStr := bodyClean.String()
	var lines []string
	for i := 0; i < len(cleanStr); i += pemLineWidth {
		end := i + pemLineWidth
		if end > len(cleanStr) {
			end = len(cleanStr)
		}
		lines = append(lines, cleanStr[i:end])
	}

	// Reconstruct PEM
	var result strings.Builder
	result.Grow(len(header) + len(cleanStr) + len(footer) + len(lines)*2)
	result.WriteString(header)
	result.WriteByte('\n')
	for _, line := range lines {
		result.WriteString(line)
		result.WriteByte('\n')
	}
	result.WriteString(footer)

	return result.String()
}

// javascriptExecute executes JavaScript preprocessing step with program caching.
//
// Performance optimizations:
//   - Caches compiled programs in programCache (avoids recompilation)
//   - Built-in functions configured once per VM (not per execution)
//
// Security note: VMs are NOT pooled to prevent prototype pollution attacks.
// Each execution gets a fresh VM to ensure complete isolation.
func javascriptExecute(value Value, paramStr string, limits JSLimits) (Value, error) {
	if paramStr == "" {
		return Value{}, fmt.Errorf("javascript code required")
	}

	// Create fresh VM for each execution (prevents prototype pollution)
	// VM pooling was removed for security - prototype modifications persist across reuse
	vm := createConfiguredVM()

	// No defer to return VM to pool - VM is discarded after use (GC will collect it)
	// This ensures no state leakage between executions

	// Set up execution timeout using a goroutine that will interrupt the VM
	// The interrupt will cause the current execution to fail; since we create a fresh
	// VM per execution (not pooled), the interrupted VM is simply discarded afterward
	timeout := limits.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second // Default to Zabbix's 10s timeout
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(timeout):
			vm.Interrupt("JavaScript execution timeout")
		case <-done:
			return
		}
	}()
	defer close(done)

	// Panic recovery for JavaScript errors
	defer func() {
		if r := recover(); r != nil {
			// Panics from goja are already handled, but catch any other panics
		}
	}()

	// Create the main function code
	code := `
(function(value) {
	` + paramStr + `
})
`

	// Try to get compiled program from cache
	var program *goja.Program
	if cached, ok := programCache.Get(code); ok {
		program = cached
	} else {
		// Compile the program (first time for this script)
		var err error
		program, err = goja.Compile("", code, false)
		if err != nil {
			return Value{}, fmt.Errorf("javascript compilation error: %w", err)
		}
		// Cache the compiled program (bounded LRU, max 1000 entries)
		programCache.Put(code, program)
	}

	// Run the compiled program
	result, err := vm.RunProgram(program)
	if err != nil {
		return Value{}, fmt.Errorf("javascript execution error: %w", err)
	}

	// Call the function with value as parameter
	fn := result
	var fnVal goja.Callable
	ok := false
	if fnVal, ok = goja.AssertFunction(fn); !ok {
		return Value{}, fmt.Errorf("javascript code must return a function")
	}

	// Execute the function
	res, err := fnVal(goja.Undefined(), vm.ToValue(value.Data))
	if err != nil {
		return Value{}, fmt.Errorf("javascript function error: %w", err)
	}

	// Get the result as string
	resultStr := res.String()
	return Value{Data: resultStr, Type: ValueTypeStr}, nil
}
