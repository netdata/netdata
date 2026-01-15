import type { Deferred } from '../utils.js';
import type { HeadendClosedEvent } from './types.js';

export const signalHeadendClosed = (
  closedSignaled: boolean,
  closeDeferred: Deferred<HeadendClosedEvent>,
  event: HeadendClosedEvent,
): boolean => {
  if (closedSignaled) return true;
  closeDeferred.resolve(event);
  return true;
};
