import http from 'node:http';
import net, { type AddressInfo } from 'node:net';

import type { HeadendContext } from '../headends/types.js';

import { AgentRegistry } from '../agent-registry.js';
import { RestHeadend } from '../headends/rest-headend.js';

const isAddressInfo = (value: ReturnType<net.Server['address']>): value is AddressInfo => {
  if (value === null || typeof value !== 'object') return false;
  if (!Object.prototype.hasOwnProperty.call(value, 'port')) return false;
  const candidate = value as { port: unknown };
  return typeof candidate.port === 'number';
};

const allocatePort = async (): Promise<number> => {
  return await new Promise<number>((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!isAddressInfo(address)) {
        reject(new Error('unable to allocate port'));
        return;
      }
      server.close((err) => {
        if (err instanceof Error) {
          reject(err);
          return;
        }
        resolve(address.port);
      });
    });
    server.on('error', reject);
  });
};

async function run(): Promise<void> {
  const port = await allocatePort();
  const registry = new AgentRegistry([]);
  const headend = new RestHeadend(registry, { port });
  const shutdownController = new AbortController();
  const stopRef = { stopping: false };
  const context: HeadendContext = {
    log: () => { /* no-op for smoke test */ },
    shutdownSignal: shutdownController.signal,
    stopRef,
  };
  await headend.start(context);

  const status = await new Promise<number>((resolve, reject) => {
    const req = http.get({ host: '127.0.0.1', port, path: '/health' }, (res) => {
      res.resume();
      res.on('end', () => { resolve(res.statusCode ?? 0); });
    });
    req.on('error', reject);
  });
  if (status !== 200) {
    throw new Error(`expected 200 from /health, received ${String(status)}`);
  }

  stopRef.stopping = true;
  shutdownController.abort();
  await headend.stop();
  const closed = await headend.closed;
  if (closed.reason !== 'stopped' || !closed.graceful) {
    throw new Error('headend did not close gracefully');
  }
}

void run()
  .then(() => {   console.log('smoke-rest-headend ok'); })
  .catch((err: unknown) => {
    const message = err instanceof Error ? err.message : String(err);
     
    console.error('smoke-rest-headend failed', message);
    process.exit(1);
  });
