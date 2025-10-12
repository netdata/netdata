import Ajv from 'ajv';

import type { MCPTool, RestToolConfig } from '../types.js';
import type { ToolExecuteOptions, ToolExecuteResult } from './types.js';

import { warn } from '../utils.js';

import { ToolProvider } from './types.js';

export class RestProvider extends ToolProvider {
  readonly kind = 'rest' as const;
  private readonly tools = new Map<string, RestToolConfig>();
  private readonly ajv = new Ajv({ allErrors: true, strict: false });

  /**
   * Serialize a value into query parameter format supporting nested structures
   * e.g., { email: 'test@example.com' } => 'email=test%40example.com'
   * e.g., [{ email: 'test@example.com' }] => '[0][email]=test%40example.com'
   */
  private serializeQueryParam(key: string, value: unknown, prefix = ''): string[] {
    const fullKey = prefix.length > 0 ? `${prefix}[${key}]` : key;

    if (value === null || value === undefined) {
      return [];
    }

    if (Array.isArray(value)) {
      const params: string[] = [];
      value.forEach((item, index) => {
        if (this.isPlainObject(item)) {
          // For array of objects, serialize each object's properties
          Object.entries(item).forEach(([k, v]) => {
            params.push(...this.serializeQueryParam(k, v, `${fullKey}[${String(index)}]`));
          });
        } else {
          // For array of primitives
          const itemStr = typeof item === 'string' || typeof item === 'number' || typeof item === 'boolean' ? String(item) : '';
          params.push(`${fullKey}[${String(index)}]=${encodeURIComponent(itemStr)}`);
        }
      });
      return params;
    }

    if (this.isPlainObject(value)) {
      const params: string[] = [];
      Object.entries(value).forEach(([k, v]) => {
        params.push(...this.serializeQueryParam(k, v, fullKey));
      });
      return params;
    }

    // Primitive value
    const valueStr = typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean' ? String(value) : '';
    return [`${fullKey}=${encodeURIComponent(valueStr)}`];
  }

  private isPlainObject(val: unknown): val is Record<string, unknown> {
    return val !== null && typeof val === 'object' && !Array.isArray(val);
  }

  constructor(public readonly id: string, config?: Record<string, RestToolConfig>) { 
    super();
    if (config !== undefined) {
      Object.entries(config).forEach(([name, def]) => { this.tools.set(name, def); });
    }
  }

  listTools(): MCPTool[] {
    const out: MCPTool[] = [];
    this.tools.forEach((cfg, name) => {
      out.push({ name: this.exposedName(name), description: cfg.description, inputSchema: cfg.parametersSchema });
    });
    return out;
  }

  hasTool(exposed: string): boolean { return this.tools.has(this.internalName(exposed)); }

  async execute(exposed: string, parameters: Record<string, unknown>, opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const start = Date.now();
    const id = this.internalName(exposed);
    const cfg = this.tools.get(id);
    if (cfg === undefined) throw new Error(`REST tool not found: ${exposed}`);

    // Validate parameters
    try {
      const validate = this.ajv.compile(cfg.parametersSchema);
      const ok = validate(parameters);
      if (!ok) {
      const list = Array.isArray(validate.errors) ? validate.errors : [];
      const errs = list.map((e) => {
        const inst = typeof e.instancePath === 'string' ? e.instancePath : '';
        const msg = typeof e.message === 'string' ? e.message : '';
        return `${inst} ${msg}`.trim();
      }).join('; ');
        throw new Error(`Invalid arguments: ${errs}`);
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      throw new Error(`Validation error: ${msg}`);
    }

    const method = cfg.method.toUpperCase();
    let url = this.substituteUrl(cfg.url, parameters);
    
    // Handle complex query parameters if marked
    const hasComplexQueryParams = (cfg as { hasComplexQueryParams?: boolean }).hasComplexQueryParams;
    const queryParamNames = (cfg as { queryParamNames?: string[] }).queryParamNames;
    
    if (hasComplexQueryParams === true && Array.isArray(queryParamNames)) {
      // Extract query params from parameters that should be serialized in complex format
      const queryParams: string[] = [];
      
      queryParamNames.forEach((paramName) => {
        if (Object.prototype.hasOwnProperty.call(parameters, paramName)) {
          let value = parameters[paramName];
          
          // Special case: if value is a plain object (not an array), wrap it in an array
          // This handles cases like Encharge API where objects need array notation
          if (this.isPlainObject(value)) {
            value = [value];
          }
          
          // Serialize this parameter with complex support
          const serialized = this.serializeQueryParam(paramName, value);
          queryParams.push(...serialized);
        }
      });
      
      if (queryParams.length > 0) {
        // Add query params to URL
        const separator = url.includes('?') ? '&' : '?';
        url = `${url}${separator}${queryParams.join('&')}`;
      }
    }
    
    // Apply header templating from parameters
    const headers = new Headers();
    const rawHeaders = cfg.headers ?? {};
    Object.entries(rawHeaders).forEach(([k, v]) => {
      const vv = typeof v === 'string' ? this.substituteString(v, parameters) : String(v);
      headers.set(k, vv);
    });

    // Log the exact API call being made for debugging or tracing
    const shouldTrace = opts?.trace === true || process.env.DEBUG_REST_CALLS === 'true' || process.env.TRACE_REST === 'true';
    if (shouldTrace) {
      const headersObj: Record<string, string> = {};
      headers.forEach((value, key) => {
        // Redact sensitive headers
        if (key.toLowerCase() === 'authorization' || key.toLowerCase() === 'x-api-key') {
          headersObj[key] = value.substring(0, 30) + '...[REDACTED]';
        } else {
          headersObj[key] = value;
        }
      });
      
      console.error(`[TRC] REST API ${exposed}`);
      console.error(`  Method: ${method}`);
      console.error(`  URL: ${url}`);
      console.error(`  Headers: ${JSON.stringify(headersObj, null, 2)}`);
    }

    // Prepare body if templated
    let body: string | undefined;
    if (cfg.bodyTemplate !== undefined) {
      const built = this.buildBody(cfg.bodyTemplate as unknown, parameters);
      body = built;
      if (!headers.has('content-type')) headers.set('content-type', 'application/json');
      
      // Log request body if present  
      if (shouldTrace) {
        let bodyPreview = body;
        if (body.length > 500) {
          bodyPreview = body.substring(0, 500) + '...[TRUNCATED]';
        }
        console.error(`  Body: ${bodyPreview}`);
      }
    }

    // Respect per-call timeout when provided
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const timeoutMs = opts?.timeoutMs;
    if (typeof timeoutMs === 'number' && Number.isFinite(timeoutMs) && timeoutMs > 0) {
      timer = setTimeout(() => { try { controller.abort(); } catch (e) { warn(`rest abort failed: ${e instanceof Error ? e.message : String(e)}`); } }, Math.trunc(timeoutMs));
    }

    const requestInit: RequestInit = { method, headers, signal: controller.signal };
    if (body !== undefined) requestInit.body = body;

    let result = '';
    let errorMsg: string | undefined;
    try {
      if (cfg.streaming?.mode === 'json-stream') {
        const res = await fetch(url, requestInit);
        if (!res.ok || res.body === null) {
          const text = await this.safeText(res);
          errorMsg = `HTTP ${String(res.status)}: ${text}`;
        } else {
          result = await this.consumeJsonStream(res, cfg, controller.signal);
        }
      } else {
        const res = await fetch(url, requestInit);
        const text = await res.text();
        if (!res.ok) {
          errorMsg = `HTTP ${String(res.status)}: ${text}`;
        } else {
          result = text;
        }
      }
    } catch (e) {
      errorMsg = e instanceof Error ? e.message : String(e);
    } finally {
      try { if (timer !== undefined) clearTimeout(timer); } catch (e) { warn(`rest clearTimeout failed: ${e instanceof Error ? e.message : String(e)}`); }
    }

    const latency = Date.now() - start;
    
    if (errorMsg !== undefined) {
      return { ok: false, error: errorMsg, latencyMs: latency, kind: this.kind, providerId: this.id };
    }
    
    return { ok: true, result, latencyMs: latency, kind: this.kind, providerId: this.id };
  }

  private async consumeJsonStream(res: Response, cfg: RestToolConfig, signal?: AbortSignal): Promise<string> {
    const dec = new TextDecoder('utf-8');
    const body = res.body;
    if (body === null) throw new Error('No response body');
    const reader = body.getReader();
    const prefix = cfg.streaming?.linePrefix ?? '';
    const disc = cfg.streaming?.discriminatorField ?? 'type';
    const doneVal = cfg.streaming?.doneValue ?? 'done';
    const ansField = cfg.streaming?.answerField ?? 'answer';
    const tokVal = cfg.streaming?.tokenValue ?? 'token';
    const tokField = cfg.streaming?.tokenField ?? 'content';
    let buffer = '';
    let tokens = '';
    // eslint-disable-next-line functional/no-loop-statements, @typescript-eslint/no-unnecessary-condition
    while (true) {
      if (signal?.aborted === true) throw new Error('Request aborted');
      const { done, value } = await reader.read();
      if (done) break;
      buffer += dec.decode(value, { stream: true });
      const lines = buffer.split(/\r?\n/);
      buffer = lines.pop() ?? '';
      // eslint-disable-next-line functional/no-loop-statements
      for (const raw of lines) {
        const line = raw.startsWith(prefix) ? raw.slice(prefix.length).trim() : raw.trim();
        if (line.length === 0 || line === '[DONE]') continue;
        try {
          const obj = JSON.parse(line) as Record<string, unknown>;
          const rec0: Record<string, unknown> = obj;
          const dv = rec0[disc];
          const kind = typeof dv === 'string' ? dv : '';
          if (kind === doneVal) {
            const rec: Record<string, unknown> = obj;
            const av = rec[ansField];
            if (typeof av === 'string') return av;
            return tokens;
          }
          if (kind === tokVal) {
            const rec2: Record<string, unknown> = obj;
            const tv = rec2[tokField];
            if (typeof tv === 'string') tokens += tv;
          }
        } catch {
          // ignore non-JSON lines
        }
      }
    }
    return tokens;
  }

  private substitute(template: unknown, parameters: Record<string, unknown>): unknown {
    if (typeof template === 'string') {
      return template.replace(/\$\{parameters\.([^}]+)\}/g, (_m, name: string) => {
        const has = Object.prototype.hasOwnProperty.call(parameters, name);
        const v = has ? parameters[name] : undefined;
        return typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean' ? String(v) : '';
      });
    }
    if (Array.isArray(template)) return template.map((v) => this.substitute(v, parameters));
    if (template !== null && typeof template === 'object') {
      const out: Record<string, unknown> = {};
      Object.entries(template as Record<string, unknown>).forEach(([k, v]) => { out[k] = this.substitute(v, parameters); });
      return out;
    }
    return template;
  }

  private substituteString(template: string, parameters: Record<string, unknown>): string {
    return template.replace(/\$\{parameters\.([^}]+)\}/g, (_m, name: string) => {
      const has = Object.prototype.hasOwnProperty.call(parameters, name);
      const v = has ? parameters[name] : undefined;
      return typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean' ? String(v) : '';
    });
  }

  private substituteUrl(template: string, parameters: Record<string, unknown>): string {
    return template.replace(/\$\{parameters\.([^}]+)\}/g, (_m, name: string) => {
      const has = Object.prototype.hasOwnProperty.call(parameters, name);
      const v = has ? parameters[name] : undefined;
      const s = (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') ? String(v) : '';
      try { return encodeURIComponent(s); } catch { return s; }
    });
  }

  private buildBody(template: unknown, parameters: Record<string, unknown>): string {
    if (typeof template === 'string') {
      // If the template is a single placeholder like ${parameters.body}, pass the raw value
      const m = /^\$\{parameters\.([^}]+)\}$/.exec(template);
      if (m !== null) {
        const key = m[1];
        const hasKey = Object.prototype.hasOwnProperty.call(parameters, key);
        const v = hasKey ? parameters[key] : undefined;
        return JSON.stringify(v);
      }
      const substituted = this.substituteString(template, parameters);
      return substituted;
    }
    const resolved = this.substitute(template, parameters);
    return JSON.stringify(resolved);
  }

  private exposedName(name: string): string { return `rest__${name}`; }
  private internalName(exposed: string): string { return exposed.startsWith('rest__') ? exposed.slice('rest__'.length) : exposed; }

  private async safeText(res: Response): Promise<string> { try { return await res.text(); } catch { return ''; } }
}
