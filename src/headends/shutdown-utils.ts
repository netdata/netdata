export interface StopRef {
  stopping: boolean;
}

export const handleHeadendShutdown = (stopRef: StopRef | undefined, closeSockets: () => void): void => {
  if (stopRef !== undefined) {
    stopRef.stopping = true;
  }
  closeSockets();
};
