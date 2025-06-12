// Chat configuration management module

// Default configuration schema
const DEFAULT_CONFIG = {
    model: {
        provider: 'anthropic',
        id: 'claude-3-5-sonnet-latest',
        params: {
            temperature: 1,
            topP: 1,
            maxTokens: 4096,
            seed: {
                enabled: false,
                value: Math.floor(Math.random() * 1000000)
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
            enabled: false,
            forgetAfterConclusions: 1
        },
        cacheControl: {
            enabled: false,
            strategy: 'smart'
        },
        titleGeneration: {
            enabled: true,
            model: null
        }
    },
    mcpServer: 'http://localhost:5173'
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

// Validate configuration structure
export function validateConfig(config) {
    if (!config || typeof config !== 'object') {
        return false;
    }
    
    // Validate model structure
    if (!config.model || !config.model.provider || !config.model.id || !config.model.params) {
        return false;
    }
    
    // Validate model params
    const params = config.model.params;
    if (typeof params.temperature !== 'number' || params.temperature < 0 || params.temperature > 2) {
        return false;
    }
    if (typeof params.topP !== 'number' || params.topP < 0 || params.topP > 1) {
        return false;
    }
    if (typeof params.maxTokens !== 'number' || params.maxTokens < 1) {
        return false;
    }
    
    // Validate optimisation structure
    return !(!config.optimisation || typeof config.optimisation !== 'object');
}

// Migrate old configuration format to new format
export function migrateConfig(oldConfig) {
    if (!oldConfig) {
        return createDefaultConfig();
    }
    
    // If already in new format, validate and return
    if (oldConfig.model && oldConfig.optimisation) {
        return validateConfig(oldConfig) ? oldConfig : createDefaultConfig();
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
            model: oldConfig.secondaryModel ? {
                provider: 'anthropic',
                id: oldConfig.secondaryModel,
                params: {
                    ...FEATURE_MODEL_DEFAULTS.toolSummarisation,
                    seed: { enabled: false, value: Math.floor(Math.random() * 1000000) }
                }
            } : null
        };
    }
    
    if (oldConfig.autoSummarization) {
        newConfig.optimisation.autoSummarisation = {
            enabled: oldConfig.autoSummarization.enabled || false,
            triggerPercent: oldConfig.autoSummarization.triggerPercent || 50,
            model: oldConfig.secondaryModel ? {
                provider: 'anthropic',
                id: oldConfig.secondaryModel,
                params: {
                    ...FEATURE_MODEL_DEFAULTS.autoSummarisation,
                    seed: { enabled: false, value: Math.floor(Math.random() * 1000000) }
                }
            } : null
        };
    }
    
    if (oldConfig.toolMemory) {
        newConfig.optimisation.toolMemory = {
            enabled: oldConfig.toolMemory.enabled || false,
            forgetAfterConclusions: oldConfig.toolMemory.forgetAfterConclusions || 1
        };
    }
    
    if (oldConfig.cacheControl) {
        newConfig.optimisation.cacheControl = {
            enabled: oldConfig.cacheControl.enabled || false,
            strategy: oldConfig.cacheControl.strategy || 'smart'
        };
    }
    
    if (oldConfig.mcpServer) {
        newConfig.mcpServer = oldConfig.mcpServer;
    }
    
    return newConfig;
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
        localStorage.setItem(key, JSON.stringify(config));
        // Also save as last config
        saveLastConfig(config);
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
            temperature: params.temperature ?? 1,
            topP: params.topP ?? 1,
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
    
    // Validate the final config
    if (!validateConfig(config)) {
        throw new Error('Invalid configuration created from options');
    }
    
    return config;
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
