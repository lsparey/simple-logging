export function formatBytes(bytes: bigint): string {
  if (bytes < 1024n) return `${bytes} B`;

  const units = ['KB', 'MB', 'GB', 'TB'];
  let value = Number(bytes);
  let unitIndex = -1;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  const precision = value >= 10 ? 1 : 2;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
}
