import Ajv from 'ajv';

import type { MCPTool, RestToolConfig } from './types.js';

// Lightweight REST tool executor. It builds requests from templates and returns raw text.
export class RestToolManager {
  private tools = new Map<string, RestToolConfig>();
  private ajv = new Ajv({ allErrors: true, strict: false });

  constructor(config?: Record<string, RestToolConfig>) {
    if (config !== undefined) {
      Object.entries(config).forEach(([name, def]) => {
        this.tools.set(name, def);
      });
    }
  }

  getAllTools(): MCPTool[] {
    const out: MCPTool[] = [];
    this.tools.forEach((cfg, name) => {
      out.push({
        name: this.exposedName(name),
        description: cfg.description,
        inputSchema: cfg.argsSchema,
      });
    });
    return out;
  }

  hasTool(exposedName: string): boolean {
    const id = this.internalName(exposedName);
    return this.tools.has(id);
  }

  async execute(exposedName: string, args: Record<string, unknown>): Promise<string> {
    const id = this.internalName(exposedName);
    const cfg = this.tools.get(id);
    if (cfg === undefined) throw new Error(`REST tool not found: ${exposedName}`);

    // Validate args
    try {
      const validate = this.ajv.compile(cfg.argsSchema);
      const ok = validate(args);
      if (!ok) {
        const list = Array.isArray(validate.errors) ? validate.errors : [];
        const errs = list.map((e) => {
          const path = typeof e.instancePath === 'string' ? e.instancePath : '';
          const msg = typeof e.message === 'string' ? e.message : '';
          return `${path} ${msg}`.trim();
        }).join('; ');
        throw new Error(`Invalid arguments: ${errs}`);
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      throw new Error(`Validation error: ${msg}`);
    }

    const method = cfg.method.toUpperCase();
    const url = cfg.url;
    const headers = new Headers(cfg.headers ?? {});

    // Prepare body if templated
    let body: string | undefined;
    if (cfg.bodyTemplate !== undefined) {
      const resolved = this.substitute(cfg.bodyTemplate, args);
      body = JSON.stringify(resolved);
      if (!headers.has('content-type')) headers.set('content-type', 'application/json');
    }

    const requestInit: RequestInit = { method, headers };
    if (body !== undefined) requestInit.body = body;

    // Streaming (JSON stream over SSE or lines)
    if (cfg.streaming?.mode === 'json-stream') {
      const res = await fetch(url, requestInit);
      if (!res.ok || res.body === null) {
        const text = await safeText(res);
        throw new Error(`HTTP ${String(res.status)}: ${text}`);
      }
      return await this.consumeJsonStream(res, cfg);
    }

    // Plain request – return body as text
    const res = await fetch(url, requestInit);
    const text = await res.text();
    if (!res.ok) throw new Error(`HTTP ${String(res.status)}: ${text}`);
    return text;
  }

  private async consumeJsonStream(res: Response, cfg: RestToolConfig): Promise<string> {
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
          const discKey: string = disc;
          const dv = obj[discKey];
          const kind = typeof dv === 'string' ? dv : '';
          if (kind === doneVal) {
            const ansKey: string = ansField;
            const ans = obj[ansKey];
            if (typeof ans === 'string') return ans;
            return tokens; // fallback to accumulated tokens
          }
          if (kind === tokVal) {
            const tokKey: string = tokField;
            const t = obj[tokKey];
            if (typeof t === 'string') tokens += t;
          }
        } catch {
          // ignore non-JSON lines
        }
      }
    }
    // Stream ended without done; return whatever tokens we saw
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
      Object.entries(template as Record<string, unknown>).forEach(([k, v]) => {
        out[k] = this.substitute(v, args);
      });
      return out;
    }
    return template;
  }

  private exposedName(name: string): string { return `rest__${name}`; }
  private internalName(exposed: string): string { return exposed.startsWith('rest__') ? exposed.slice('rest__'.length) : exposed; }
}

async function safeText(res: Response): Promise<string> {
  try { return await res.text(); } catch { return ''; }
}

// No name normalization: preserve user-defined tool ids (may include '-')
