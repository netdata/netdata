/**
 * WebSocket transport implementation for MCP
 */

import WebSocket from 'ws';

import type { Transport } from '@modelcontextprotocol/sdk/shared/transport.js';
import type { JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';

export class WebSocketTransport implements Transport {
  private ws: WebSocket;
  private messageHandlers = new Set<(message: JSONRPCMessage) => void>();
  private closeHandlers = new Set<() => void>();
  private errorHandlers = new Set<(error: Error) => void>();

  constructor(private url: string, headers?: Record<string, string>) {
    this.ws = new WebSocket(url, { headers });
    this.setupEventHandlers();
  }

  private setupEventHandlers(): void {
    this.ws.on('message', (data: WebSocket.RawData) => {
      try {
        let dataStr: string;
        if (data instanceof Buffer) {
          dataStr = data.toString('utf8');
        } else if (typeof data === 'string') {
          dataStr = data;
        } else if (Array.isArray(data)) {
          dataStr = Buffer.concat(data).toString('utf8');
        } else {
          // Handle any other WebSocket.RawData types (like ArrayBuffer)
          dataStr = data instanceof ArrayBuffer ? 
            Buffer.from(data).toString('utf8') : 
            String(data);
        }
        const message = JSON.parse(dataStr) as JSONRPCMessage;
        this.messageHandlers.forEach((handler) => {
          handler(message);
        });
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : String(error);
        const err = new Error(`Failed to parse WebSocket message: ${errorMessage}`);
        this.errorHandlers.forEach((handler) => {
          handler(err);
        });
      }
    });

    this.ws.on('close', () => {
      this.closeHandlers.forEach((handler) => {
        handler();
      });
    });

    this.ws.on('error', (error) => {
      const err = new Error(`WebSocket error: ${error.message}`);
      this.errorHandlers.forEach((handler) => {
        handler(err);
      });
    });
  }

  async start(): Promise<void> {
    if (this.ws.readyState === WebSocket.OPEN) {
      return;
    }

    return new Promise((resolve, reject) => {
      const onOpen = () => {
        this.ws.removeListener('error', onError);
        resolve();
      };

      const onError = (error: Error) => {
        this.ws.removeListener('open', onOpen);
        reject(new Error(`Failed to connect to WebSocket ${this.url}: ${error.message}`));
      };

      this.ws.once('open', onOpen);
      this.ws.once('error', onError);
    });
  }

  async send(message: JSONRPCMessage): Promise<void> {
    if (this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('WebSocket is not connected');
    }

    return new Promise((resolve, reject) => {
      this.ws.send(JSON.stringify(message), (error) => {
        if (error != null) {
          reject(new Error(`Failed to send WebSocket message: ${error.message}`));
        } else {
          resolve();
        }
      });
    });
  }

  async close(): Promise<void> {
    if (this.ws.readyState === WebSocket.CLOSED) {
      return;
    }

    return new Promise((resolve) => {
      if (this.ws.readyState === WebSocket.OPEN) {
        this.ws.close();
      }

      const onClose = () => {
        resolve();
      };

      this.ws.once('close', onClose);
      
      // Force close after timeout
      setTimeout(() => {
        if (this.ws.readyState !== WebSocket.CLOSED) {
          this.ws.terminate();
          resolve();
        }
      }, 5000);
    });
  }

  onMessage(handler: (message: JSONRPCMessage) => void): void {
    this.messageHandlers.add(handler);
  }

  onClose(handler: () => void): void {
    this.closeHandlers.add(handler);
  }

  onError(handler: (error: Error) => void): void {
    this.errorHandlers.add(handler);
  }
}

/**
 * Create and initialize a WebSocket transport
 */
export async function createWebSocketTransport(
  url: string,
  headers?: Record<string, string>
): Promise<WebSocketTransport> {
  const transport = new WebSocketTransport(url, headers);
  await transport.start();
  return transport;
}