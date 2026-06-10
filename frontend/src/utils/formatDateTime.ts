const dateTimeFormatter = new Intl.DateTimeFormat(undefined, {
  dateStyle: 'medium',
  timeStyle: 'short',
});

const dateFormatter = new Intl.DateTimeFormat(undefined, {
  dateStyle: 'medium',
});

export function formatDateTime(unixMs: bigint): string {
  return dateTimeFormatter.format(new Date(Number(unixMs)));
}

export function formatDate(unixMs: bigint): string {
  return dateFormatter.format(new Date(Number(unixMs)));
}
