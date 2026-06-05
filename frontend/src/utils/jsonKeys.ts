export function candidateJsonKeys(lines: string[], sampleSize = 100): string[] {
  const prefixJsonRe = /^(\S+) \[\S+\] ([\s\S]*)$/;
  const keyCounts = new Map<string, number>();
  let jsonLineCount = 0;

  const sample = lines.length > sampleSize
    ? lines.slice(0, Math.ceil(sampleSize / 2)).concat(lines.slice(-Math.floor(sampleSize / 2)))
    : lines;

  for (const line of sample) {
    const m = prefixJsonRe.exec(line);
    const payload = m ? m[2] : line;
    const trimmed = payload.trimStart();
    if (trimmed[0] !== '{') continue;
    try {
      const obj = JSON.parse(trimmed) as Record<string, unknown>;
      if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) continue;
      jsonLineCount++;
      for (const key of Object.keys(obj)) {
        keyCounts.set(key, (keyCounts.get(key) ?? 0) + 1);
      }
    } catch { /* not json */ }
  }

  if (jsonLineCount === 0) return [];

  return [...keyCounts.entries()]
    .filter(([, count]) => count === jsonLineCount)
    .map(([key]) => key)
    .sort();
}
