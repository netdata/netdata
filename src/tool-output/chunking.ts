interface Chunk {
  index: number;
  text: string;
  startByte: number;
  endByte: number;
}

const findSafeUtf8Boundary = (buffer: Buffer, targetOffset: number): number => {
  if (targetOffset >= buffer.length) return buffer.length;
  if (targetOffset <= 0) return 0;
  let offset = targetOffset;
  // eslint-disable-next-line functional/no-loop-statements -- byte-wise scan
  while (offset > 0 && (buffer[offset] & 0xc0) === 0x80) {
    offset -= 1;
  }
  return offset;
};

const findSafeUtf8BoundaryAfter = (buffer: Buffer, targetOffset: number): number => {
  if (targetOffset >= buffer.length) return buffer.length;
  if (targetOffset <= 0) return 0;
  let offset = targetOffset;
  // eslint-disable-next-line functional/no-loop-statements -- byte-wise scan
  while (offset < buffer.length && (buffer[offset] & 0xc0) === 0x80) {
    offset += 1;
  }
  return offset;
};

export function splitIntoChunks(text: string, chunkBytes: number, overlapPercent: number): Chunk[] {
  if (chunkBytes <= 0) return [];
  const buffer = Buffer.from(text, 'utf8');
  if (buffer.length === 0) return [];
  const overlapBytes = Math.max(0, Math.floor((chunkBytes * overlapPercent) / 100));
  const chunks: Chunk[] = [];
  let start = 0;
  let index = 0;
  // eslint-disable-next-line functional/no-loop-statements -- chunk iteration
  while (start < buffer.length) {
    let end = Math.min(start + chunkBytes, buffer.length);
    end = findSafeUtf8Boundary(buffer, end);
    if (end <= start) {
      end = Math.min(start + chunkBytes, buffer.length);
    }
    const safeEnd = Math.max(start, end);
    const textChunk = buffer.subarray(start, safeEnd).toString('utf8');
    chunks.push({ index, text: textChunk, startByte: start, endByte: safeEnd });
    index += 1;
    if (safeEnd >= buffer.length) break;
    const nextStartRaw = safeEnd - overlapBytes;
    const nextStart = findSafeUtf8BoundaryAfter(buffer, Math.max(0, nextStartRaw));
    if (nextStart <= start) {
      start = safeEnd;
    } else {
      start = nextStart;
    }
  }
  return chunks;
}

export function computeChunkCount(totalTokens: number, chunkTokenBudget: number): number {
  if (chunkTokenBudget <= 0) return Number.POSITIVE_INFINITY;
  if (totalTokens <= 0) return 1;
  return Math.max(1, Math.ceil(totalTokens / chunkTokenBudget));
}
