/**
 * Safety Limits Configuration
 * 
 * Centralized configuration for all safety protections to prevent
 * runaway assistant behavior and excessive resource usage.
 */

export const SAFETY_LIMITS = {
    // Maximum consecutive tool iterations before stopping with error
    MAX_CONSECUTIVE_TOOL_ITERATIONS: 10,
    
    // Maximum concurrent tools per request before stopping with error
    MAX_CONCURRENT_TOOLS_PER_REQUEST: 15,
    
    // Maximum JSON request size in bytes before stopping with error
    MAX_REQUEST_SIZE_BYTES: 200 * 1024, // 200 KiB
    
    // Error messages for each safety violation
    ERRORS: {
        TOO_MANY_ITERATIONS: 'Assistant exceeded maximum consecutive tool iterations limit. This conversation has been stopped to prevent runaway behavior.',
        TOO_MANY_CONCURRENT_TOOLS: 'Request contains too many concurrent tools. This conversation has been stopped to prevent resource exhaustion.',
        REQUEST_TOO_LARGE: 'Request size exceeds maximum limit. This conversation has been stopped to prevent memory issues.'
    }
};

/**
 * Safety checker class to validate requests before sending to LLM
 */
export class SafetyChecker {
    constructor() {
        this.iterationCounts = new Map(); // chatId -> iteration count
    }
    
    /**
     * Reset iteration count for a chat (call when user sends new message)
     */
    resetIterations(chatId) {
        this.iterationCounts.set(chatId, 0);
    }
    
    /**
     * Increment iteration count for a chat
     */
    incrementIterations(chatId) {
        const current = this.iterationCounts.get(chatId) || 0;
        this.iterationCounts.set(chatId, current + 1);
        return current + 1;
    }
    
    /**
     * Get current iteration count for a chat
     */
    getIterationCount(chatId) {
        return this.iterationCounts.get(chatId) || 0;
    }
    
    /**
     * Check if consecutive tool iterations limit would be exceeded
     */
    checkIterationLimit(chatId) {
        const count = this.getIterationCount(chatId);
        if (count >= SAFETY_LIMITS.MAX_CONSECUTIVE_TOOL_ITERATIONS) {
            throw new SafetyLimitError('ITERATIONS', SAFETY_LIMITS.ERRORS.TOO_MANY_ITERATIONS);
        }
    }
    
    /**
     * Check if concurrent tools limit would be exceeded
     */
    checkConcurrentToolsLimit(toolCalls) {
        if (!Array.isArray(toolCalls)) return;
        
        if (toolCalls.length > SAFETY_LIMITS.MAX_CONCURRENT_TOOLS_PER_REQUEST) {
            throw new SafetyLimitError('CONCURRENT_TOOLS', SAFETY_LIMITS.ERRORS.TOO_MANY_CONCURRENT_TOOLS);
        }
    }
    
    /**
     * Check if request size limit would be exceeded
     */
    checkRequestSizeLimit(requestData) {
        const jsonString = JSON.stringify(requestData);
        const sizeInBytes = new TextEncoder().encode(jsonString).length;
        
        if (sizeInBytes > SAFETY_LIMITS.MAX_REQUEST_SIZE_BYTES) {
            throw new SafetyLimitError('REQUEST_SIZE', SAFETY_LIMITS.ERRORS.REQUEST_TOO_LARGE);
        }
    }
    
    /**
     * Comprehensive safety check before sending request
     */
    validateRequest(chatId, requestData, toolCalls = []) {
        // Check iteration limit (only if this is a tool-using iteration)
        if (toolCalls && toolCalls.length > 0) {
            this.checkIterationLimit(chatId);
        }
        
        // Check concurrent tools limit
        this.checkConcurrentToolsLimit(toolCalls);
        
        // Check request size limit
        this.checkRequestSizeLimit(requestData);
    }
}

/**
 * Custom error class for safety limit violations
 */
export class SafetyLimitError extends Error {
    constructor(type, message) {
        super(message);
        this.name = 'SafetyLimitError';
        this.type = type; // 'ITERATIONS', 'CONCURRENT_TOOLS', 'REQUEST_SIZE'
        this.isRetryable = false; // These errors should never allow retry
    }
}