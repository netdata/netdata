import crypto from 'node:crypto';

import express from 'express';

import type { SimpleAskRequestBody, SimpleAskResponseBody } from './types.js';
import { SessionManager } from './session-manager.js';

const BEARER_PREFIX = 'Bearer ';

const bearerAuth = (req: any, allowedKeys: string[]): { ok: boolean; error?: string } => {
  if (!Array.isArray(allowedKeys) || allowedKeys.length === 0) return { ok: false, error: 'server-misconfigured: no API keys' };
  const header = req.get('authorization');
  if (header === undefined || !header.startsWith(BEARER_PREFIX)) return { ok: false, error: 'missing bearer token' };
  const token = header.slice(BEARER_PREFIX.length).trim();
  if (token.length === 0) return { ok: false, error: 'empty bearer token' };
  const a = Buffer.from(token);
  // eslint-disable-next-line functional/no-loop-statements
  for (const key of allowedKeys) {
    const b = Buffer.from(key);
    if (a.length !== b.length) continue;
    try {
      if (crypto.timingSafeEqual(a, b)) return { ok: true };
    } catch {
      // ignore
    }
  }
  return { ok: false, error: 'unauthorized' };
};

export function buildApiRouter(sessions: SessionManager, opts: { bearerKeys: string[], systemPrompt: string }): any {
  const router = express.Router();
  router.use(express.json({ limit: '512kb' }));

  router.post('/ask', async (req: any, res: any, next: any) => {
    try {
      const auth = bearerAuth(req, opts.bearerKeys);
      if (!auth.ok) {
        res.status(401).json({ error: auth.error });
        return;
      }

      const body: unknown = req.body;
      if (!body || typeof body !== 'object' || typeof (body as Record<string, unknown>).prompt !== 'string') {
        res.status(400).json({ error: 'invalid body: { prompt: string } required' });
        return;
      }

      const { prompt } = body as SimpleAskRequestBody;

      const runId = sessions.startRun({ source: 'api', threadTsOrSessionId: crypto.randomUUID() }, opts.systemPrompt, prompt, undefined);

      const started = Date.now();
      const timeoutMs = 60_000;
      // eslint-disable-next-line functional/no-loop-statements
      while (Date.now() - started < timeoutMs) {
        const meta = sessions.getRun(runId);
        if (meta && meta.status !== 'running') {
          const out = sessions.getOutput(runId) ?? '';
          const payload: SimpleAskResponseBody = {
            runId,
            status: meta.status === 'succeeded' ? 'succeeded' : 'failed',
            text: meta.status === 'succeeded' ? out : undefined,
            error: meta.status === 'failed' ? meta.error ?? 'unknown error' : undefined
          };
          res.status(200).json(payload);
          return;
        }
        // eslint-disable-next-line no-promise-executor-return
        await new Promise((r) => setTimeout(r, 250));
      }

      const payload: SimpleAskResponseBody = { runId, status: 'running' };
      res.status(202).json(payload);
      return;
    } catch (err) {
      return next(err);
    }
  });

  router.get('/runs/:id', (req: any, res: any) => {
    const auth = bearerAuth(req, opts.bearerKeys);
    if (!auth.ok) {
      res.status(401).json({ error: auth.error });
      return;
    }
    const runIdParam = req.params.id;
    if (typeof runIdParam !== 'string' || runIdParam.length === 0) {
      res.status(400).json({ error: 'invalid run id' });
      return;
    }
    const meta = sessions.getRun(runIdParam);
    if (!meta) {
      res.status(404).json({ error: 'not found' });
      return;
    }
    res.status(200).json({
      runId: meta.runId,
      status: meta.status,
      startedAt: meta.startedAt,
      updatedAt: meta.updatedAt,
      error: meta.error
    });
  });

  router.get('/runs/:id/tree', (req: any, res: any) => {
    const auth = bearerAuth(req, opts.bearerKeys);
    if (!auth.ok) {
      res.status(401).json({ error: auth.error });
      return;
    }
    const runIdParam = req.params.id;
    if (typeof runIdParam !== 'string' || runIdParam.length === 0) {
      res.status(400).json({ error: 'invalid run id' });
      return;
    }
    const meta = sessions.getRun(runIdParam);
    if (!meta) {
      res.status(404).json({ error: 'not found' });
      return;
    }
    const tree = sessions.getOpTree(runIdParam);
    if (!tree) {
      res.status(404).json({ error: 'tree not available' });
      return;
    }
    // Basic redaction for secrets in request/response payloads
    const redact = (obj: unknown): unknown => {
      if (obj === null || obj === undefined) return obj;
      if (Array.isArray(obj)) return (obj as unknown[]).map(redact);
      if (typeof obj === 'object') {
        const out: Record<string, unknown> = {};
        for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
          const lower = k.toLowerCase();
          if (lower === 'authorization' || lower === 'x-api-key' || lower === 'api-key' || lower === 'apikey' || lower === 'password' || lower === 'token' || lower === 'secret') {
            out[k] = '[REDACTED]';
          } else {
            out[k] = redact(v as unknown);
          }
        }
        return out;
      }
      return obj;
    };
    res.status(200).json(redact(tree));
  });

  return router;
}