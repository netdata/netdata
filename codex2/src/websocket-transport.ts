import WebSocket from 'ws';

import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js';
import type { JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';

export class WebSocketTransport implements Transport {
  private ws: WebSocket;
  private messageHandlers = new Set<(message: JSONRPCMessage) => void>();
  private closeHandlers = new Set<() => void>();
  private errorHandlers = new Set<(error: Error) => void>();

  constructor(private url: string, private headers?: Record<string, string>) {
    this.ws = new WebSocket(url, { headers });
    this.setup();
  }

  private setup(): void {
    this.ws.on('message', (data: WebSocket.RawData) => {
      try {
        let text = '';
        if (typeof data === 'string') text = data;
        else if (Buffer.isBuffer(data)) text = data.toString('utf8');
        else if (Array.isArray(data)) text = Buffer.concat(data).toString('utf8');
        const msg = JSON.parse(text) as JSONRPCMessage;
        this.messageHandlers.forEach((h) => { h(msg); });
      } catch (e) {
        const err = new Error('Failed to parse WebSocket message: ' + (e instanceof Error ? e.message : String(e)));
        this.errorHandlers.forEach((h) => { h(err); });
      }
    });
    this.ws.on('close', () => { this.closeHandlers.forEach((h) => { h(); }); });
    this.ws.on('error', (e) => {
      const err = new Error('WebSocket error: ' + (e instanceof Error ? e.message : String(e)));
      this.errorHandlers.forEach((h) => { h(err); });
    });
  }

  async start(): Promise<void> {
    if (this.ws.readyState === WebSocket.OPEN) return;
    await new Promise<void>((resolve, reject) => {
      const onOpen = () => { this.ws.off('error', onError); resolve(); };
      const onError = (e: Error) => { this.ws.off('open', onOpen); reject(new Error('Failed to connect to WebSocket ' + this.url + ': ' + e.message)); };
      this.ws.once('open', onOpen);
      this.ws.once('error', onError);
    });
  }

  async send(message: JSONRPCMessage): Promise<void> {
    if (this.ws.readyState !== WebSocket.OPEN) throw new Error('WebSocket is not connected');
    await new Promise<void>((resolve, reject) => {
      this.ws.send(JSON.stringify(message), (err: Error | undefined | null) => { if (err != null) { reject(err); } else { resolve(); } });
    });
  }

  async close(): Promise<void> {
    if (this.ws.readyState === WebSocket.CLOSED) return;
    await new Promise<void>((resolve) => {
      if (this.ws.readyState === WebSocket.OPEN) { this.ws.close(); }
      const done = (): void => { resolve(); };
      this.ws.once('close', done);
      setTimeout((): void => {
        if (this.ws.readyState !== WebSocket.CLOSED) {
          this.ws.terminate();
          resolve();
        }
      }, 5000);
    });
  }

  onMessage(handler: (message: JSONRPCMessage) => void): void { this.messageHandlers.add(handler); }
  onClose(handler: () => void): void { this.closeHandlers.add(handler); }
  onError(handler: (error: Error) => void): void { this.errorHandlers.add(handler); }
}

export async function createWebSocketTransport(url: string, headers?: Record<string, string>): Promise<WebSocketTransport> {
  const t = new WebSocketTransport(url, headers);
  await t.start();
  return t;
}
