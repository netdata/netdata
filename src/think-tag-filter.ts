interface ThinkSplit {
  content: string;
  thinking: string;
}

const THINK_OPEN = '<think>';
const THINK_CLOSE = '</think>';

export const stripLeadingThinkBlock = (input: string): { stripped: string; removed: boolean } => {
  if (input.length === 0) return { stripped: input, removed: false };
  const match = /^\s*<think>[\s\S]*?<\/think>/.exec(input);
  if (match === null) return { stripped: input, removed: false };
  return { stripped: input.slice(match[0].length), removed: true };
};

export class ThinkTagStreamFilter {
  private state: 'prefix' | 'inside_think' | 'content' = 'prefix';
  private prefixBuffer = '';
  private thinkCandidate = '';
  private thinkBuffer = '';

  process(chunk: string): ThinkSplit {
    let contentOut = '';
    let thinkingOut = '';
    // eslint-disable-next-line functional/no-loop-statements
    for (const char of chunk) {
      if (this.state === 'prefix') {
        if (this.thinkCandidate.length === 0 && /\s/.test(char)) {
          this.prefixBuffer += char;
          continue;
        }

        this.thinkCandidate += char;
        if (this.thinkCandidate === THINK_OPEN) {
          this.state = 'inside_think';
          this.thinkCandidate = '';
          this.prefixBuffer = '';
          continue;
        }

        if (THINK_OPEN.startsWith(this.thinkCandidate)) {
          continue;
        }

        contentOut += this.prefixBuffer + this.thinkCandidate;
        this.prefixBuffer = '';
        this.thinkCandidate = '';
        this.state = 'content';
        continue;
      }

      if (this.state === 'inside_think') {
        this.thinkBuffer += char;
        if (this.thinkBuffer.endsWith(THINK_CLOSE)) {
          const withoutClose = this.thinkBuffer.slice(0, -THINK_CLOSE.length);
          if (withoutClose.length > 0) {
            thinkingOut += withoutClose;
          }
          this.thinkBuffer = '';
          this.state = 'content';
          continue;
        }

        if (this.thinkBuffer.length > THINK_CLOSE.length) {
          thinkingOut += this.thinkBuffer[0];
          this.thinkBuffer = this.thinkBuffer.slice(1);
        }

        if (this.thinkBuffer.length > 100000) {
          thinkingOut += this.thinkBuffer.slice(0, this.thinkBuffer.length - THINK_CLOSE.length);
          this.thinkBuffer = this.thinkBuffer.slice(-THINK_CLOSE.length);
        }
        continue;
      }

      contentOut += char;
    }

    return { content: contentOut, thinking: thinkingOut };
  }
}
