const dateTimeFormatter = new Intl.DateTimeFormat(undefined, {
  dateStyle: 'medium',
  timeStyle: 'short',
});

export function formatDateTime(unixMs: bigint): string {
  return dateTimeFormatter.format(new Date(Number(unixMs)));
}
