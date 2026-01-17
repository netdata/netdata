export type StopReason = 'stop' | 'abort' | 'shutdown';

export interface StopRef {
  stopping: boolean;
  reason?: StopReason;
}

export const handleHeadendShutdown = (stopRef: StopRef | undefined, closeSockets: () => void): void => {
  if (stopRef !== undefined) {
    stopRef.stopping = true;
  }
  closeSockets();
};
