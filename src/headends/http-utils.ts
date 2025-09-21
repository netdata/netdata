import type http from 'node:http';

const DEFAULT_BODY_LIMIT = 1024 * 1024; // 1 MiB

export class HttpError extends Error {
  public readonly statusCode: number;
  public readonly code: string;

  public constructor(statusCode: number, code: string, message: string) {
    super(message);
    this.statusCode = statusCode;
    this.code = code;
  }
}

export const readBody = async (req: http.IncomingMessage, limit = DEFAULT_BODY_LIMIT): Promise<string> => {
  const chunks: Buffer[] = [];
  let total = 0;
  return await new Promise<string>((resolve, reject) => {
    req.on('data', (chunk: Buffer | string) => {
      const buf = typeof chunk === 'string' ? Buffer.from(chunk) : chunk;
      total += buf.length;
      if (total > limit) {
        reject(new HttpError(413, 'payload_too_large', 'Request body too large'));
        req.destroy();
        return;
      }
      chunks.push(buf);
    });
    req.on('end', () => { resolve(Buffer.concat(chunks).toString('utf8')); });
    req.on('error', (err) => { reject(err instanceof Error ? err : new Error(String(err))); });
  });
};

export const readJson = async <T = unknown>(req: http.IncomingMessage, limit = DEFAULT_BODY_LIMIT): Promise<T> => {
  const text = await readBody(req, limit);
  if (text.length === 0) {
    throw new HttpError(400, 'empty_body', 'Request body is required');
  }
  try {
    return JSON.parse(text) as T;
  } catch (err) {
    throw new HttpError(400, 'invalid_json', err instanceof Error ? err.message : 'Invalid JSON body');
  }
};

export const writeJson = (res: http.ServerResponse, statusCode: number, payload: unknown, headers?: Record<string, string>): void => {
  if (res.writableEnded || res.writableFinished) return;
  const body = JSON.stringify(payload);
  res.statusCode = statusCode;
  res.setHeader('Content-Type', 'application/json; charset=utf-8');
  if (headers !== undefined) {
    Object.entries(headers).forEach(([key, value]) => {
      res.setHeader(key, value);
    });
  }
  res.setHeader('Content-Length', Buffer.byteLength(body));
  res.end(body);
};

export const writeSseChunk = (res: http.ServerResponse, payload: unknown): void => {
  if (res.writableEnded || res.writableFinished) return;
  res.write(`data: ${JSON.stringify(payload)}\n\n`);
};

export const writeSseDone = (res: http.ServerResponse): void => {
  if (res.writableEnded || res.writableFinished) return;
  res.write('data: [DONE]\n\n');
};
