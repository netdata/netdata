const aiSdkGlobals = globalThis as typeof globalThis & {
  AI_SDK_LOG_WARNINGS?: boolean;
};

const debugEnabled = process.env.DEBUG === 'true';

if (debugEnabled) {
  delete aiSdkGlobals.AI_SDK_LOG_WARNINGS;
} else if (typeof aiSdkGlobals.AI_SDK_LOG_WARNINGS !== 'boolean') {
  aiSdkGlobals.AI_SDK_LOG_WARNINGS = false;
}

export function updateAiSdkWarningPreference(traceEnabled: boolean): void {
  if (process.env.DEBUG === 'true' || traceEnabled) {
    delete aiSdkGlobals.AI_SDK_LOG_WARNINGS;
    return;
  }
  aiSdkGlobals.AI_SDK_LOG_WARNINGS = false;
}
