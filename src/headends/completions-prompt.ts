interface PromptSectionsInput {
  systemParts?: string[];
  historyParts?: string[];
  lastUser: string;
}

const SYSTEM_PREFIX = 'System context:\n';
const HISTORY_PREFIX = 'Conversation so far:\n';
const USER_PREFIX = 'User request:\n';

export const buildPromptSections = ({ systemParts, historyParts, lastUser }: PromptSectionsInput): string => {
  const sections: string[] = [];
  if (Array.isArray(systemParts) && systemParts.length > 0) {
    sections.push(`${SYSTEM_PREFIX}${systemParts.join('\n')}`);
  }
  if (Array.isArray(historyParts) && historyParts.length > 0) {
    sections.push(`${HISTORY_PREFIX}${historyParts.join('\n')}`);
  }
  sections.push(`${USER_PREFIX}${lastUser}`);
  return sections.join('\n\n');
};
