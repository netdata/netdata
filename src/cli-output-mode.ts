export const resolveCliOutputMode = (chatOption: unknown): 'chat' | 'agentic' | undefined => {
  if (chatOption === true) return 'chat';
  if (chatOption === false) return 'agentic';
  return 'agentic';
};
