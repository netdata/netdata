// Chat configuration management module

// Default configuration schema
const DEFAULT_CONFIG = {
    model: {
        provider: 'anthropic',
        id: 'claude-sonnet-4-20250514',
        params: {
            temperature: 0.7,
            topP: 0.9,
            maxTokens: 4096,
            seed: {
                enabled: false,
                value: 216569
            }
        }
    },
    optimisation: {
        toolSummarisation: {
            enabled: false,
            thresholdKiB: 20,
            model: null
        },
        autoSummarisation: {
            enabled: false,
            triggerPercent: 50,
            model: null
        },
        toolMemory: {
            enabled: true,
            forgetAfterConclusions: 0
        },
        cacheControl: 'system',
        titleGeneration: {
            enabled: true,
            model: {
                provider: 'google',
                id: 'gemini-1.5-flash-8b',
                params: {
                    temperature: 0.7,
                    topP: 0.9,
                    maxTokens: 100,
                    seed: {
                        enabled: false,
                        value: 872763
                    }
                }
            }
        }
    },
    mcpServer: 'prod_aws_parent0'
};

// Feature-specific default model parameters
const FEATURE_MODEL_DEFAULTS = {
    toolSummarisation: {
        temperature: 0.3,
        topP: 0.9,
        maxTokens: 1024
    },
    autoSummarisation: {
        temperature: 0.5,
        topP: 0.95,
        maxTokens: 2048
    },
    titleGeneration: {
        temperature: 0.7,
        topP: 0.9,
        maxTokens: 100
    }
};

// Create a new default configuration
export function createDefaultConfig() {
    return JSON.parse(JSON.stringify(DEFAULT_CONFIG));
}

// Validate and normalize configuration - always returns a valid config
export function validateConfig(config) {
    // Start with a default config as base
    const validConfig = createDefaultConfig();
    
    // If no config provided, return default
    if (!config || typeof config !== 'object') {
        console.error('[validateConfig] Invalid input: config is not an object, using default');
        return validConfig;
    }
    
    // Copy model settings if valid
    if (config.model && typeof config.model === 'object') {
        validConfig.model.provider = config.model.provider || validConfig.model.provider;
        validConfig.model.id = config.model.id || validConfig.model.id;
        
        if (!config.model.provider) {
            console.warn('[validateConfig] model.provider is missing');
        }
        if (!config.model.id) {
            console.warn('[validateConfig] model.id is missing');
        }
        
        // Copy model params if valid
        if (config.model.params && typeof config.model.params === 'object') {
            const params = config.model.params;
            
            // Temperature
            if (typeof params.temperature === 'number' && params.temperature >= 0 && params.temperature <= 2) {
                validConfig.model.params.temperature = params.temperature;
            } else {
                console.warn('[validateConfig] Invalid temperature:', params.temperature, '- using default:', validConfig.model.params.temperature);
            }
            
            // TopP
            if (typeof params.topP === 'number' && params.topP >= 0 && params.topP <= 1) {
                validConfig.model.params.topP = params.topP;
            } else {
                console.warn('[validateConfig] Invalid topP:', params.topP, '- using default:', validConfig.model.params.topP);
            }
            
            // MaxTokens
            if (typeof params.maxTokens === 'number' && params.maxTokens >= 1) {
                validConfig.model.params.maxTokens = params.maxTokens;
            } else {
                console.warn('[validateConfig] Invalid maxTokens:', params.maxTokens, '- using default:', validConfig.model.params.maxTokens);
            }
            
            // Seed
            if (params.seed && typeof params.seed === 'object') {
                validConfig.model.params.seed = {
                    enabled: !!params.seed.enabled,
                    value: typeof params.seed.value === 'number' ? params.seed.value : Math.floor(Math.random() * 1000000)
                };
                if (typeof params.seed.value !== 'number') {
                    console.warn('[validateConfig] Invalid seed value:', params.seed.value, '- generating random');
                }
            }
        } else {
            console.warn('[validateConfig] model.params missing or invalid');
        }
    } else {
        console.warn('[validateConfig] model section missing or invalid');
    }
    
    // Copy optimisation settings if valid
    if (config.optimisation && typeof config.optimisation === 'object') {
        Object.assign(validConfig.optimisation, config.optimisation);
    } else {
        console.warn('[validateConfig] optimisation section missing or invalid');
    }
    
    // Copy mcpServer if valid
    if (config.mcpServer && typeof config.mcpServer === 'string') {
        validConfig.mcpServer = config.mcpServer;
    } else {
        console.warn('[validateConfig] mcpServer missing or invalid:', config.mcpServer);
    }
    
    // Normalize the config
    normalizeConfig(validConfig);
    
    return validConfig;
}

// Normalize configuration values to ensure consistency
export function normalizeConfig(config) {
    // Ensure all optimisation sections exist
    const opt = config.optimisation;
    
    // Tool Memory normalization
    if (!opt.toolMemory || typeof opt.toolMemory !== 'object') {
        console.warn('[normalizeConfig] toolMemory missing, creating default');
        opt.toolMemory = { enabled: false, forgetAfterConclusions: 1 };
    } else if (opt.toolMemory.forgetAfterConclusions === undefined || 
               opt.toolMemory.forgetAfterConclusions === null ||
               typeof opt.toolMemory.forgetAfterConclusions !== 'number' ||
               opt.toolMemory.forgetAfterConclusions < 0 ||
               opt.toolMemory.forgetAfterConclusions > 5) {
        console.error('[normalizeConfig] Invalid toolMemory.forgetAfterConclusions:', opt.toolMemory.forgetAfterConclusions, '- disabling tool memory');
        opt.toolMemory.enabled = false;
        opt.toolMemory.forgetAfterConclusions = 1;
    }
    
    // Title Generation normalization - ensure maxTokens is capped at 100
    if (!opt.titleGeneration || typeof opt.titleGeneration !== 'object') {
        console.warn('[normalizeConfig] titleGeneration missing, creating default');
        opt.titleGeneration = { enabled: true, model: null };
    } else if (opt.titleGeneration.model && opt.titleGeneration.model.params) {
        // Cap title generation tokens at 100
        if (opt.titleGeneration.model.params.maxTokens > 100) {
            console.warn('[normalizeConfig] titleGeneration.model.params.maxTokens too high:', opt.titleGeneration.model.params.maxTokens, '- capping at 100');
            opt.titleGeneration.model.params.maxTokens = 100;
        }
    }
    
    // Tool Summarisation normalization
    if (!opt.toolSummarisation || typeof opt.toolSummarisation !== 'object') {
        console.warn('[normalizeConfig] toolSummarisation missing, creating default');
        opt.toolSummarisation = { enabled: false, thresholdKiB: 20, model: null };
    }
    
    // Auto Summarisation normalization
    if (!opt.autoSummarisation || typeof opt.autoSummarisation !== 'object') {
        console.warn('[normalizeConfig] autoSummarisation missing, creating default');
        opt.autoSummarisation = { enabled: false, triggerPercent: 50, model: null };
    }
    
    // Cache Control normalization
    if (!opt.cacheControl || typeof opt.cacheControl !== 'string') {
        // Don't warn for missing cacheControl - it's expected for old chats
        opt.cacheControl = 'all-off';
    } else if (!['all-off', 'system', 'cached'].includes(opt.cacheControl)) {
        console.warn('[normalizeConfig] Invalid cacheControl value:', opt.cacheControl, '- using default');
        opt.cacheControl = 'all-off';
    }
    
    // MCP Server validation happens in app.js where the server list is available
    // We just pass through whatever value is in the config
}

// Migrate old configuration format to new format
export function migrateConfig(oldConfig) {
    if (!oldConfig) {
        return createDefaultConfig();
    }
    
    // If already in new format, validate and return
    if (oldConfig.model && oldConfig.optimisation) {
        return validateConfig(oldConfig);
    }
    
    // Migration from old format
    const newConfig = createDefaultConfig();
    
    // Migrate model settings
    if (oldConfig.primaryModel) {
        const [provider, ...idParts] = oldConfig.primaryModel.split(':');
        if (!provider || idParts.length === 0) {
            throw new Error(`Invalid model format in old config: "${oldConfig.primaryModel}". Expected "provider:model"`);
        }
        newConfig.model.provider = provider;
        newConfig.model.id = idParts.join(':');
    }
    
    // Migrate optimization settings
    if (oldConfig.toolSummarization) {
        newConfig.optimisation.toolSummarisation = {
            enabled: oldConfig.toolSummarization.enabled || false,
            thresholdKiB: Math.floor((oldConfig.toolSummarization.threshold || 50000) / 1024),
            model: oldConfig.secondaryModel ? modelConfigFromString(oldConfig.secondaryModel, FEATURE_MODEL_DEFAULTS.toolSummarisation) : null
        };
    }
    
    if (oldConfig.autoSummarization) {
        newConfig.optimisation.autoSummarisation = {
            enabled: oldConfig.autoSummarization.enabled || false,
            triggerPercent: oldConfig.autoSummarization.triggerPercent || 50,
            model: oldConfig.secondaryModel ? modelConfigFromString(oldConfig.secondaryModel, FEATURE_MODEL_DEFAULTS.autoSummarisation) : null
        };
    }
    
    if (oldConfig.toolMemory) {
        newConfig.optimisation.toolMemory = {
            enabled: oldConfig.toolMemory.enabled,
            forgetAfterConclusions: oldConfig.toolMemory.forgetAfterConclusions
        };
    }
    
    if (oldConfig.cacheControl) {
        // Migrate old cache control format to new format
        if (oldConfig.cacheControl.enabled) {
            // If enabled, use cached mode (keeps current strategy behavior)
            newConfig.optimisation.cacheControl = 'cached';
        } else {
            // If disabled, use all-off mode
            newConfig.optimisation.cacheControl = 'all-off';
        }
    }
    
    if (oldConfig.mcpServer) {
        newConfig.mcpServer = oldConfig.mcpServer;
    }
    
    // Validate and normalize the migrated config
    return validateConfig(newConfig);
}

// Load configuration for a specific chat
export function loadChatConfig(chatId) {
    const key = `chatConfig_${chatId}`;
    try {
        const stored = localStorage.getItem(key);
        if (stored) {
            const config = JSON.parse(stored);
            return migrateConfig(config);
        }
    } catch (e) {
        console.error('Error loading chat config:', e);
    }
    
    // If no config exists, use last config or default
    return getLastConfig();
}

// Save configuration for a specific chat
export function saveChatConfig(chatId, config) {
    const key = `chatConfig_${chatId}`;
    try {
        // Validate and normalize before saving
        const validConfig = validateConfig(config);
        localStorage.setItem(key, JSON.stringify(validConfig));
        // Also save as last config
        saveLastConfig(validConfig);
    } catch (e) {
        console.error('Error saving chat config:', e);
    }
}

// Get the last used configuration
export function getLastConfig() {
    try {
        const stored = localStorage.getItem('lastChatConfig');
        if (stored) {
            const config = JSON.parse(stored);
            return migrateConfig(config);
        }
    } catch (e) {
        console.error('Error loading last config:', e);
    }
    
    return createDefaultConfig();
}

// Save configuration as the last used
export function saveLastConfig(config) {
    try {
        localStorage.setItem('lastChatConfig', JSON.stringify(config));
    } catch (e) {
        console.error('Error saving last config:', e);
    }
}

// Merge two configurations (shallow merge of top-level properties)
export function mergeConfigs(base, override) {
    const merged = JSON.parse(JSON.stringify(base));
    
    if (override.model) {
        merged.model = override.model;
    }
    
    if (override.optimisation) {
        Object.assign(merged.optimisation, override.optimisation);
    }
    
    if (override.mcpServer !== undefined) {
        merged.mcpServer = override.mcpServer;
    }
    
    return merged;
}

// Get model configuration for a specific feature
export function getModelConfig(config, feature = null) {
    if (!feature) {
        return config.model;
    }
    
    const featureConfig = config.optimisation[feature];
    if (featureConfig && featureConfig.model) {
        return featureConfig.model;
    }
    
    // Return primary model with feature-specific defaults
    const primaryModel = JSON.parse(JSON.stringify(config.model));
    if (FEATURE_MODEL_DEFAULTS[feature]) {
        Object.assign(primaryModel.params, FEATURE_MODEL_DEFAULTS[feature]);
    }
    
    return primaryModel;
}

// Convert model config to string format for API
export function modelConfigToString(modelConfig) {
    if (!modelConfig) return null;
    return `${modelConfig.provider}:${modelConfig.id}`;
}

// Create model config from string format
export function modelConfigFromString(modelString, params = {}) {
    if (!modelString) return null;
    
    const [provider, ...idParts] = modelString.split(':');
    if (!provider || idParts.length === 0) {
        throw new Error(`Invalid model string format: "${modelString}". Expected "provider:model"`);
    }
    
    return {
        provider,
        id: idParts.join(':'),
        params: {
            temperature: params.temperature ?? 0.7,
            topP: params.topP ?? 0.9,
            maxTokens: params.maxTokens ?? 4096,
            seed: params.seed ?? { enabled: false, value: Math.floor(Math.random() * 1000000) }
        }
    };
}

// Create a complete configuration from options
export function createConfigFromOptions(options = {}) {
    let config;
    
    // If a config is provided, use it as base
    if (options.config) {
        config = options.config;
    } else {
        // Otherwise create default config
        config = createDefaultConfig();
    }
    
    // If a model string is provided, update the config
    if (options.model) {
        const parts = options.model.split(':');
        if (parts.length > 1) {
            // Format: "provider:model"
            config.model = {
                provider: parts[0],
                id: parts.slice(1).join(':'),
                params: config.model.params
            };
        } else {
            // No provider specified - THIS IS AN ERROR!
            throw new Error(`Model string "${options.model}" is missing provider prefix. Expected format: "provider:model"`);
        }
    }
    
    // Update MCP server if provided
    if (options.mcpServerId) {
        config.mcpServer = options.mcpServerId;
    }
    
    // Validate and normalize the final config
    return validateConfig(config);
}

// Get optimizer settings from config
export function getOptimizerSettings(config, llmProviderFactory) {
    if (!config) {
        throw new Error('Config is required for optimizer settings');
    }
    
    return {
        ...config,
        llmProviderFactory: config.optimisation.toolSummarisation.enabled ? llmProviderFactory : undefined
    };
}

// Get display name for a model (just the model ID without provider)
export function getModelDisplayName(modelString) {
    if (!modelString) return '';
    
    // If it's already a config object, extract the id
    if (typeof modelString === 'object' && modelString.id) {
        return modelString.id;
    }
    
    // If it's a string with provider:model format
    if (typeof modelString === 'string' && modelString.includes(':')) {
        const parts = modelString.split(':');
        return parts.slice(1).join(':');
    }
    
    // Otherwise return as-is
    return modelString;
}

// Get provider from model string
export function getProviderFromModelString(modelString) {
    if (!modelString) return null;
    
    // If it's already a config object, extract the provider
    if (typeof modelString === 'object' && modelString.provider) {
        return modelString.provider;
    }
    
    // If it's a string with provider:model format
    if (typeof modelString === 'string' && modelString.includes(':')) {
        return modelString.split(':')[0];
    }
    
    return null;
}

// Get model string from chat object
export function getChatModelString(chat) {
    if (!chat) return null;
    
    // If chat has config with model, use that
    if (chat.config && chat.config.model) {
        return modelConfigToString(chat.config.model);
    }
    
    // Fallback to legacy chat.model if it exists
    if (chat.model) {
        return chat.model;
    }
    
    return null;
}
