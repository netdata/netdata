const aiSdkGlobals = globalThis as typeof globalThis & {
  AI_SDK_LOG_WARNINGS?: boolean;
};

if (typeof aiSdkGlobals.AI_SDK_LOG_WARNINGS !== 'boolean') {
  aiSdkGlobals.AI_SDK_LOG_WARNINGS = false;
}

export {};
