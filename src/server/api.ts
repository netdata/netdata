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
      if (!auth.ok) return res.status(401).json({ error: auth.error });

      const body: unknown = req.body;
      if (!body || typeof body !== 'object' || typeof (body as Record<string, unknown>).prompt !== 'string') {
        return res.status(400).json({ error: 'invalid body: { prompt: string } required' });
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
          return res.status(200).json(payload);
        }
        // eslint-disable-next-line no-promise-executor-return
        await new Promise((r) => setTimeout(r, 250));
      }

      const payload: SimpleAskResponseBody = { runId, status: 'running' };
      return res.status(202).json(payload);
    } catch (err) {
      return next(err);
    }
  });

  router.get('/runs/:id', (req: any, res: any) => {
    const auth = bearerAuth(req, opts.bearerKeys);
    if (!auth.ok) return res.status(401).json({ error: auth.error });
    const runIdParam = req.params?.id;
    if (typeof runIdParam !== 'string' || runIdParam.length === 0) {
      return res.status(400).json({ error: 'invalid run id' });
    }
    const meta = sessions.getRun(runIdParam);
    if (!meta) return res.status(404).json({ error: 'not found' });
    return res.status(200).json({
      runId: meta.runId,
      status: meta.status,
      startedAt: meta.startedAt,
      updatedAt: meta.updatedAt,
      error: meta.error
    });
  });

  return router;
}
