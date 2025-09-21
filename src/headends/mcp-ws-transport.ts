import { randomUUID } from 'node:crypto';

import { JSONRPCMessageSchema, type JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';

import type WebSocket from 'ws';

/**
 * Minimal WebSocket transport compatible with the MCP server Protocol.
 */
export class McpWebSocketServerTransport {
  public onmessage?: (message: JSONRPCMessage) => void;
  public onclose?: () => void;
  public onerror?: (error: unknown) => void;
  public readonly sessionId: string;

  private readonly socket: WebSocket;

  public constructor(socket: WebSocket) {
    this.socket = socket;
    this.sessionId = randomUUID();
  }

  public start(): Promise<void> {
    this.socket.on('message', (raw) => {
      try {
        let text: string;
        if (typeof raw === 'string') {
          text = raw;
        } else if (raw instanceof ArrayBuffer) {
          text = Buffer.from(raw).toString('utf8');
        } else if (Array.isArray(raw)) {
          text = Buffer.concat(raw).toString('utf8');
        } else if (Buffer.isBuffer(raw)) {
          text = raw.toString('utf8');
        } else {
          text = String(raw);
        }
        const parsed = JSONRPCMessageSchema.parse(JSON.parse(text));
        this.onmessage?.(parsed);
      } catch (err) {
        this.onerror?.(err);
      }
    });
    this.socket.on('close', () => {
      this.onclose?.();
    });
    this.socket.on('error', (err) => {
      this.onerror?.(err);
    });
    return Promise.resolve();
  }

  public close(): Promise<void> {
    try {
      this.socket.close();
    } catch (err) {
      this.onerror?.(err);
    }
    return Promise.resolve();
  }

  public async send(message: JSONRPCMessage): Promise<void> {
    await new Promise<void>((resolve, reject) => {
      try {
        this.socket.send(JSON.stringify(message), (err) => {
          if (err !== undefined) {
            reject(err instanceof Error ? err : new Error(String(err)));
            return;
          }
          resolve();
        });
      } catch (err) {
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    });
  }
}
