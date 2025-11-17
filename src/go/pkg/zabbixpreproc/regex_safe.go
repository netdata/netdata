package zabbixpreproc

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"
)

// Regex safety constants
const (
	maxRegexNestingDepth = 10              // Maximum nesting depth for groups
	defaultRegexTimeout  = 5 * time.Second // Default timeout for regex operations
	regexCacheSize       = 100             // Maximum cached patterns
)

// regexCache caches compiled regular expressions for performance
type regexCache struct {
	mu    sync.RWMutex
	cache map[string]*regexp.Regexp
	order []string // LRU tracking
}

var globalRegexCache = &regexCache{
	cache: make(map[string]*regexp.Regexp),
	order: make([]string, 0, regexCacheSize),
}

// validateRegexComplexity performs basic complexity checks on regex patterns
// maxPatternLength of 0 means no limit
func validateRegexComplexity(pattern string, maxPatternLength int) error {
	if maxPatternLength > 0 && len(pattern) > maxPatternLength {
		return fmt.Errorf("regex pattern too long: %d chars (max %d)",
			len(pattern), maxPatternLength)
	}

	// Count nesting depth of parentheses
	depth := 0
	maxDepth := 0
	escaped := false

	for _, ch := range pattern {
		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '(' {
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		} else if ch == ')' {
			depth--
			if depth < 0 {
				return fmt.Errorf("unbalanced parentheses in regex pattern")
			}
		}
	}

	if depth != 0 {
		return fmt.Errorf("unbalanced parentheses in regex pattern")
	}

	if maxDepth > maxRegexNestingDepth {
		return fmt.Errorf("regex nesting too deep: %d levels (max %d)",
			maxDepth, maxRegexNestingDepth)
	}

	return nil
}

// compileRegexSafe compiles a regex with validation and caching
// maxPatternLength of 0 means no limit (Zabbix default)
func compileRegexSafe(pattern string, maxPatternLength int) (*regexp.Regexp, error) {
	// Check cache first (read lock)
	globalRegexCache.mu.RLock()
	if re, found := globalRegexCache.cache[pattern]; found {
		globalRegexCache.mu.RUnlock()
		return re, nil
	}
	globalRegexCache.mu.RUnlock()

	// Validate complexity before compiling
	if err := validateRegexComplexity(pattern, maxPatternLength); err != nil {
		return nil, err
	}

	// Compile pattern
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	// Add to cache (write lock)
	globalRegexCache.mu.Lock()
	defer globalRegexCache.mu.Unlock()

	// Double-check cache after acquiring write lock (another goroutine might have added it)
	if cached, found := globalRegexCache.cache[pattern]; found {
		return cached, nil
	}

	// Evict oldest entry if cache is full (simple FIFO)
	if len(globalRegexCache.cache) >= regexCacheSize {
		oldest := globalRegexCache.order[0]
		delete(globalRegexCache.cache, oldest)
		globalRegexCache.order = globalRegexCache.order[1:]
	}

	// Add to cache
	globalRegexCache.cache[pattern] = re
	globalRegexCache.order = append(globalRegexCache.order, pattern)

	return re, nil
}

// matchWithTimeout runs a regex match with timeout protection
func matchWithTimeout(re *regexp.Regexp, input string, timeout time.Duration) (bool, error) {
	if timeout == 0 {
		timeout = defaultRegexTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan bool, 1)
	errChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case errChan <- fmt.Errorf("regex match panicked: %v", r):
				case <-ctx.Done():
					return // Context cancelled, exit immediately
				}
			}
		}()

		matched := re.MatchString(input)
		select {
		case resultChan <- matched:
		case <-ctx.Done():
			return // Context cancelled, exit immediately
		}
	}()

	select {
	case matched := <-resultChan:
		return matched, nil
	case err := <-errChan:
		return false, err
	case <-ctx.Done():
		return false, fmt.Errorf("regex match timeout after %v", timeout)
	}
}

// findStringSubmatchWithTimeout runs FindStringSubmatch with timeout protection
func findStringSubmatchWithTimeout(re *regexp.Regexp, input string, timeout time.Duration) ([]string, error) {
	if timeout == 0 {
		timeout = defaultRegexTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan []string, 1)
	errChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case errChan <- fmt.Errorf("regex find panicked: %v", r):
				case <-ctx.Done():
					return // Context cancelled, exit immediately
				}
			}
		}()

		matches := re.FindStringSubmatch(input)
		select {
		case resultChan <- matches:
		case <-ctx.Done():
			return // Context cancelled, exit immediately
		}
	}()

	select {
	case matches := <-resultChan:
		return matches, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("regex find timeout after %v", timeout)
	}
}

// findStringSubmatchIndexWithTimeout runs FindStringSubmatchIndex with timeout protection
func findStringSubmatchIndexWithTimeout(re *regexp.Regexp, input string, timeout time.Duration) ([]int, error) {
	if timeout == 0 {
		timeout = defaultRegexTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan []int, 1)
	errChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case errChan <- fmt.Errorf("regex find index panicked: %v", r):
				case <-ctx.Done():
					return // Context cancelled, exit immediately
				}
			}
		}()

		matches := re.FindStringSubmatchIndex(input)
		select {
		case resultChan <- matches:
		case <-ctx.Done():
			return // Context cancelled, exit immediately
		}
	}()

	select {
	case matches := <-resultChan:
		return matches, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("regex find index timeout after %v", timeout)
	}
}

// ClearRegexCache clears the global regex cache (useful for testing)
func ClearRegexCache() {
	globalRegexCache.mu.Lock()
	defer globalRegexCache.mu.Unlock()

	globalRegexCache.cache = make(map[string]*regexp.Regexp)
	globalRegexCache.order = make([]string, 0, regexCacheSize)
}
