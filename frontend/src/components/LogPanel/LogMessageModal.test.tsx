import { fireEvent, render, screen } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import LogMessageModal from './LogMessageModal.js';

const theme = createTheme();

function renderModal(line: string | null, onClose = vi.fn()) {
  return render(
    <ThemeProvider theme={theme}>
      <LogMessageModal open={line !== null} line={line} onClose={onClose} />
    </ThemeProvider>,
  );
}

beforeEach(() => {
  Object.assign(navigator, {
    clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
});

describe('LogMessageModal', () => {
  it('renders nothing visible when closed', () => {
    renderModal(null);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('shows the full line text when open', () => {
    const longLine = '2024-01-15T10:00:00Z INFO '.repeat(20) + 'the full message';
    renderModal(longLine);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText(/the full message/)).toBeInTheDocument();
  });

  it('copies the full line text to the clipboard', async () => {
    const line = 'a very long log line that would otherwise be clipped';
    renderModal(line);

    fireEvent.click(screen.getByRole('button', { name: 'Copy' }));

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(line);
    expect(await screen.findByRole('button', { name: 'Copied!' })).toBeInTheDocument();
  });

  it('calls onClose when the Close button is clicked', () => {
    const onClose = vi.fn();
    renderModal('some line', onClose);

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
