import Ajv from 'ajv';

import type { MCPTool, RestToolConfig } from '../types.js';
import type { ToolExecuteOptions, ToolExecuteResult } from './types.js';

import { ToolProvider } from './types.js';

export class RestProvider extends ToolProvider {
  readonly kind = 'rest' as const;
  private readonly tools = new Map<string, RestToolConfig>();
  private readonly ajv = new Ajv({ allErrors: true, strict: false });

  constructor(public readonly id: string, config?: Record<string, RestToolConfig>) { 
    super();
    if (config !== undefined) {
      Object.entries(config).forEach(([name, def]) => { this.tools.set(name, def); });
    }
  }

  listTools(): MCPTool[] {
    const out: MCPTool[] = [];
    this.tools.forEach((cfg, name) => {
      out.push({ name: this.exposedName(name), description: cfg.description, inputSchema: cfg.argsSchema });
    });
    return out;
  }

  hasTool(exposed: string): boolean { return this.tools.has(this.internalName(exposed)); }

  async execute(exposed: string, args: Record<string, unknown>, _opts?: ToolExecuteOptions): Promise<ToolExecuteResult> {
    const start = Date.now();
    const id = this.internalName(exposed);
    const cfg = this.tools.get(id);
    if (cfg === undefined) throw new Error(`REST tool not found: ${exposed}`);

    // Validate args
    try {
      const validate = this.ajv.compile(cfg.argsSchema);
      const ok = validate(args);
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
    const url = this.substituteUrl(cfg.url, args);
    // Apply header templating from args
    const headers = new Headers();
    const rawHeaders = cfg.headers ?? {};
    Object.entries(rawHeaders).forEach(([k, v]) => {
      const vv = typeof v === 'string' ? this.substituteString(v, args) : String(v);
      headers.set(k, vv);
    });

    // Prepare body if templated
    let body: string | undefined;
    if (cfg.bodyTemplate !== undefined) {
      const built = this.buildBody(cfg.bodyTemplate as unknown, args);
      body = built;
      if (!headers.has('content-type')) headers.set('content-type', 'application/json');
    }

    // Respect per-call timeout when provided
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const timeoutMs = _opts?.timeoutMs;
    if (typeof timeoutMs === 'number' && Number.isFinite(timeoutMs) && timeoutMs > 0) {
      timer = setTimeout(() => { try { controller.abort(); } catch { /* ignore */ } }, Math.trunc(timeoutMs));
    }

    const requestInit: RequestInit = { method, headers, signal: controller.signal };
    if (body !== undefined) requestInit.body = body;

    let result = '';
    try {
      if (cfg.streaming?.mode === 'json-stream') {
        const res = await fetch(url, requestInit);
        if (!res.ok || res.body === null) {
          const text = await this.safeText(res);
          throw new Error(`HTTP ${String(res.status)}: ${text}`);
        }
        result = await this.consumeJsonStream(res, cfg, controller.signal);
      } else {
        const res = await fetch(url, requestInit);
        const text = await res.text();
        if (!res.ok) throw new Error(`HTTP ${String(res.status)}: ${text}`);
        result = text;
      }
    } finally {
      try { if (timer !== undefined) clearTimeout(timer); } catch { /* ignore */ }
    }

    const latency = Date.now() - start;
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

  private substitute(template: unknown, args: Record<string, unknown>): unknown {
    if (typeof template === 'string') {
      return template.replace(/\$\{args\.([^}]+)\}/g, (_m, name: string) => {
        const has = Object.prototype.hasOwnProperty.call(args, name);
        const v = has ? args[name] : undefined;
        return typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean' ? String(v) : '';
      });
    }
    if (Array.isArray(template)) return template.map((v) => this.substitute(v, args));
    if (template !== null && typeof template === 'object') {
      const out: Record<string, unknown> = {};
      Object.entries(template as Record<string, unknown>).forEach(([k, v]) => { out[k] = this.substitute(v, args); });
      return out;
    }
    return template;
  }

  private substituteString(template: string, args: Record<string, unknown>): string {
    return template.replace(/\$\{args\.([^}]+)\}/g, (_m, name: string) => {
      const has = Object.prototype.hasOwnProperty.call(args, name);
      const v = has ? args[name] : undefined;
      return typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean' ? String(v) : '';
    });
  }

  private substituteUrl(template: string, args: Record<string, unknown>): string {
    return template.replace(/\$\{args\.([^}]+)\}/g, (_m, name: string) => {
      const has = Object.prototype.hasOwnProperty.call(args, name);
      const v = has ? args[name] : undefined;
      const s = (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') ? String(v) : '';
      try { return encodeURIComponent(s); } catch { return s; }
    });
  }

  private buildBody(template: unknown, args: Record<string, unknown>): string {
    if (typeof template === 'string') {
      // If the template is a single placeholder like ${args.body}, pass the raw value
      const m = /^\$\{args\.([^}]+)\}$/.exec(template);
      if (m !== null) {
        const key = m[1];
        const hasKey = Object.prototype.hasOwnProperty.call(args, key);
        const v = hasKey ? args[key] : undefined;
        return JSON.stringify(v);
      }
      const substituted = this.substituteString(template, args);
      return substituted;
    }
    const resolved = this.substitute(template, args);
    return JSON.stringify(resolved);
  }

  private exposedName(name: string): string { return `rest__${name}`; }
  private internalName(exposed: string): string { return exposed.startsWith('rest__') ? exposed.slice('rest__'.length) : exposed; }

  private async safeText(res: Response): Promise<string> { try { return await res.text(); } catch { return ''; } }
}
