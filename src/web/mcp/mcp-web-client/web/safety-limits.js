/**
 * Safety Limits Configuration
 * 
 * Centralized configuration for all safety protections to prevent
 * runaway assistant behavior and excessive resource usage.
 */

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

export const SAFETY_LIMITS = {
    // Maximum consecutive tool iterations before stopping with error
    MAX_CONSECUTIVE_TOOL_ITERATIONS: 20,
    
    // Maximum concurrent tools per request before stopping with error
    MAX_CONCURRENT_TOOLS_PER_REQUEST: 15,
    
    // Maximum JSON request size in bytes before stopping with error
    MAX_REQUEST_SIZE_BYTES: 400 * 1024, // 400 KiB
    
    // Error messages for each safety violation
    ERRORS: {
        TOO_MANY_ITERATIONS: (current, limit) => `Assistant has made ${current} consecutive tool iterations. Maximum allowed is ${limit}.`,
        TOO_MANY_CONCURRENT_TOOLS: (current, limit) => `Request contains ${current} concurrent tool calls. Maximum allowed is ${limit}.`,
        REQUEST_TOO_LARGE: (currentBytes, limitBytes) => `Request size is ${(currentBytes / 1024).toFixed(2)} KiB (${currentBytes} bytes). Maximum allowed is ${(limitBytes / 1024).toFixed(2)} KiB (${limitBytes} bytes).`
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
            const errorMsg = SAFETY_LIMITS.ERRORS.TOO_MANY_ITERATIONS(count, SAFETY_LIMITS.MAX_CONSECUTIVE_TOOL_ITERATIONS);
            console.error(`[SafetyLimit] Iteration limit exceeded for chat ${chatId}:`, errorMsg);
            throw new SafetyLimitError('ITERATIONS', errorMsg);
        }
    }
    
    /**
     * Check if concurrent tools limit would be exceeded
     */
    checkConcurrentToolsLimit(toolCalls) {
        if (!Array.isArray(toolCalls)) return;
        
        if (toolCalls.length > SAFETY_LIMITS.MAX_CONCURRENT_TOOLS_PER_REQUEST) {
            const errorMsg = SAFETY_LIMITS.ERRORS.TOO_MANY_CONCURRENT_TOOLS(toolCalls.length, SAFETY_LIMITS.MAX_CONCURRENT_TOOLS_PER_REQUEST);
            console.error(`[SafetyLimit] Concurrent tools limit exceeded:`, errorMsg);
            console.error(`[SafetyLimit] Tool calls:`, toolCalls.map(tc => tc.name || tc.function?.name));
            throw new SafetyLimitError('CONCURRENT_TOOLS', errorMsg);
        }
    }
    
    /**
     * Check if request size limit would be exceeded
     */
    checkRequestSizeLimit(requestData) {
        const jsonString = JSON.stringify(requestData);
        const sizeInBytes = new TextEncoder().encode(jsonString).length;
        
        if (sizeInBytes > SAFETY_LIMITS.MAX_REQUEST_SIZE_BYTES) {
            const errorMsg = SAFETY_LIMITS.ERRORS.REQUEST_TOO_LARGE(sizeInBytes, SAFETY_LIMITS.MAX_REQUEST_SIZE_BYTES);
            console.error(`[SafetyLimit] Request size limit exceeded:`, errorMsg);
            console.error(`[SafetyLimit] Request structure:`, {
                messages: requestData.messages?.length || 0,
                tools: requestData.tools?.length || 0,
                model: requestData.model,
                sizeBytes: sizeInBytes
            });
            throw new SafetyLimitError('REQUEST_SIZE', errorMsg);
        }
    }
    
    /**
     * Comprehensive safety check before sending request
     * @deprecated Use individual check methods instead
     */
    validateRequest(chatId, requestData, toolCalls = []) {
        // Check iteration limit (only if this is a tool-using iteration)
        if (toolCalls && toolCalls.length > 0) {
            this.checkIterationLimit(chatId);
        }
        
        // Check concurrent tools limit
        this.checkConcurrentToolsLimit(toolCalls);
        
        // NOTE: Request size check removed - now done in LLM providers
        // where the actual request is built with all fields
    }
}