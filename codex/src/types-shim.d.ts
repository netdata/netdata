declare module '@modelcontextprotocol/sdk/client/http.js' {
  export const HTTPClientTransport: unknown;
}





declare module '@openrouter/ai-sdk-provider' {
  import type { LanguageModel } from 'ai';
  export interface OpenRouterProvider {
    (model: string): LanguageModel;
    responses: (model: string) => LanguageModel;
    chat: (model: string) => LanguageModel;
  }
  export function createOpenRouter(config: {
    apiKey?: string;
    baseURL?: string;
    fetch?: typeof fetch;
    headers?: Record<string, string>;
  }): OpenRouterProvider;
}
