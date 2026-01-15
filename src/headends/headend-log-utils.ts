import type { LogEntry } from '../types.js';
import type { HeadendContext } from './types.js';

export const logHeadendEntry = (context: HeadendContext | undefined, entry: LogEntry): void => {
  if (context === undefined) return;
  context.log(entry);
};
