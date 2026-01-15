import type { Socket } from 'node:net';

export const closeSockets = (sockets: Set<Socket>, force = false): void => {
  const delay = force ? 0 : 1000;
  sockets.forEach((socket) => {
    try { socket.end(); } catch { /* ignore */ }
    const timer = setTimeout(() => { try { socket.destroy(); } catch { /* ignore */ } }, delay);
    timer.unref();
  });
};
