/**
 * Copies text to the clipboard.
 *
 * `navigator.clipboard` is only defined in secure contexts (HTTPS or
 * localhost). When the app is served over plain HTTP - as can happen when
 * accessed via an internal hostname/IP inside a cluster - `navigator.clipboard`
 * is `undefined` and calling `.writeText` throws. Fall back to the legacy
 * `document.execCommand('copy')` approach in that case.
 *
 * Returns `true` if the copy succeeded, `false` otherwise.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to the legacy approach below.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '-9999px';
  textarea.style.left = '-9999px';
  document.body.appendChild(textarea);
  textarea.select();

  let succeeded: boolean;
  try {
    succeeded = document.execCommand('copy');
  } catch {
    succeeded = false;
  } finally {
    document.body.removeChild(textarea);
  }

  return succeeded;
}
