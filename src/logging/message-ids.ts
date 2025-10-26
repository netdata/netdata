import type { LogEntry } from '../types.js';

// Registry of journald MESSAGE_IDs (UUIDs) for well-known log events.
// Each ID should remain stable across releases.
const MESSAGE_ID_REGISTRY: Partial<Record<string, string>> = {
  'agent:init': '8f8c1c67-7f2f-4a63-a632-0dbdfdd41d39',
  'agent:fin': '4cb03727-5d3a-45d7-9b85-4e6940d3d2c4',
  'agent:pricing': 'f53bb387-032c-4b79-9b0f-4c6f1ad20a41',
  'agent:limits': '50a01c4a-86b2-4d1d-ae13-3d0b4504b597',
  'agent:EXIT-FINAL-ANSWER': '6f3db0fb-a931-47cb-b060-9f4881ae9b14',
  'agent:batch': 'c9a43c6d-fd32-4687-9a72-a673a4c3c303',
  'agent:progress': 'b8e3466f-ef86-4337-8a4b-09d5f5e0be1f',
};

export function resolveMessageId(entry: LogEntry): string | undefined {
  const { remoteIdentifier } = entry;
  if (typeof remoteIdentifier !== 'string' || remoteIdentifier.length === 0) return undefined;
  const normalized = remoteIdentifier.trim();
  const direct = MESSAGE_ID_REGISTRY[normalized];
  if (direct !== undefined) return direct;
  // Tool-specific identifiers fall back to their base prefix (e.g., agent:progress)
  const prefix = normalized.split(':', 2).join(':');
  return MESSAGE_ID_REGISTRY[prefix];
}

export function getRegisteredMessageIds(): Record<string, string | undefined> {
  return { ...MESSAGE_ID_REGISTRY };
}
