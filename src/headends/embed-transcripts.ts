import fs from 'node:fs';
import path from 'node:path';
import { promisify } from 'node:util';
import { gzip as gzipCallback, gunzip as gunzipCallback } from 'node:zlib';

import { isPlainObject } from '../utils.js';

const gzip = promisify(gzipCallback);
const gunzip = promisify(gunzipCallback);

export type EmbedTranscriptRole = 'user' | 'assistant' | 'status';

export interface EmbedTranscriptEntry {
  role: EmbedTranscriptRole;
  content: string;
}

export interface EmbedTranscriptTurn {
  turn: number;
  ts: string;
  entries: EmbedTranscriptEntry[];
}

export interface EmbedTranscriptFile {
  version: 1;
  clientId: string;
  origin: 'embed';
  updatedAt: string;
  turns: EmbedTranscriptTurn[];
}

const TRANSCRIPT_VERSION = 1 as const;

const buildEmptyTranscript = (clientId: string): EmbedTranscriptFile => ({
  version: TRANSCRIPT_VERSION,
  clientId,
  origin: 'embed',
  updatedAt: new Date().toISOString(),
  turns: [],
});

const isTranscriptRole = (value: unknown): value is EmbedTranscriptRole => {
  return value === 'user' || value === 'assistant' || value === 'status';
};

const parseTranscript = (raw: string, clientId: string): EmbedTranscriptFile => {
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!isPlainObject(parsed)) return buildEmptyTranscript(clientId);
    const version = parsed.version === TRANSCRIPT_VERSION ? TRANSCRIPT_VERSION : TRANSCRIPT_VERSION;
    const turnsRaw = Array.isArray(parsed.turns) ? parsed.turns : [];
    const turns = turnsRaw
      .filter((turn): turn is Record<string, unknown> => isPlainObject(turn))
      .map((turn, index) => {
        const entriesRaw = Array.isArray(turn.entries) ? turn.entries : [];
        const entries = entriesRaw
          .filter((entry): entry is Record<string, unknown> => isPlainObject(entry))
          .map((entry) => {
            const role = isTranscriptRole(entry.role) ? entry.role : 'status';
            const content = typeof entry.content === 'string' ? entry.content : '';
            return { role, content };
          });
        return {
          turn: typeof turn.turn === 'number' && Number.isFinite(turn.turn) ? turn.turn : index + 1,
          ts: typeof turn.ts === 'string' ? turn.ts : new Date().toISOString(),
          entries,
        };
      });
    return {
      version,
      clientId: typeof parsed.clientId === 'string' ? parsed.clientId : clientId,
      origin: 'embed',
      updatedAt: typeof parsed.updatedAt === 'string' ? parsed.updatedAt : new Date().toISOString(),
      turns,
    };
  } catch {
    return buildEmptyTranscript(clientId);
  }
};

export const loadTranscript = async (filePath: string, clientId: string): Promise<EmbedTranscriptFile> => {
  try {
    const gz = await fs.promises.readFile(filePath);
    const jsonBuf = await gunzip(gz);
    return parseTranscript(jsonBuf.toString('utf8'), clientId);
  } catch {
    return buildEmptyTranscript(clientId);
  }
};

export const appendTranscriptTurn = (
  transcript: EmbedTranscriptFile,
  entries: EmbedTranscriptEntry[],
  timestamp: Date
): EmbedTranscriptFile => {
  const nextTurn = transcript.turns.length + 1;
  const next: EmbedTranscriptTurn = {
    turn: nextTurn,
    ts: timestamp.toISOString(),
    entries,
  };
  return {
    ...transcript,
    updatedAt: timestamp.toISOString(),
    turns: [...transcript.turns, next],
  };
};

export const writeTranscript = async (filePath: string, transcript: EmbedTranscriptFile): Promise<void> => {
  const dir = path.dirname(filePath);
  await fs.promises.mkdir(dir, { recursive: true });
  const json = JSON.stringify(transcript);
  const gz = await gzip(Buffer.from(json, 'utf8'));
  const tmp = `${filePath}.tmp-${String(process.pid)}-${String(Date.now())}`;
  await fs.promises.writeFile(tmp, gz);
  await fs.promises.rename(tmp, filePath);
};
