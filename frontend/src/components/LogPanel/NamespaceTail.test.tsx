import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import NamespaceTail from './NamespaceTail.js';
import LogPanel from './LogPanel.js';
import { logClient } from '../../grpc/client.js';
import { useLogStore } from '../../store/logStore.js';

vi.mock('../../grpc/client.js', () => ({
  baseUrl: 'http://localhost',
  logClient: {
    streamWorkloadLogs: vi.fn(),
    getWorkloadLogs: vi.fn(),
  },
}));
// LogList virtualises rows (needing real layout); render lines plainly.
vi.mock('./LogList.js', () => ({
  default: ({ lines }: { lines: string[] }) => (
    <div data-testid="log-list">{lines.map((l) => <div key={l}>{l}</div>)}</div>
  ),
}));

const streamWorkloadLogs = vi.mocked(logClient.streamWorkloadLogs);
const theme = createTheme();

/** A stream that yields lines, then stays open until aborted. */
function openStream(lines: string[]) {
  return (_req: unknown, opts?: { signal?: AbortSignal }) => ({
    [Symbol.asyncIterator]: async function* () {
      for (const line of lines) yield { line };
      await new Promise<void>((resolve) => opts?.signal?.addEventListener('abort', () => resolve()));
    },
  }) as unknown as ReturnType<typeof logClient.streamWorkloadLogs>;
}

beforeEach(() => {
  streamWorkloadLogs.mockReset();
  useLogStore.setState({ selectedNamespace: 'default', selectedWorkloadKind: null, selectedWorkloadName: null, lines: [], mode: 'idle' });
});

describe('NamespaceTail', () => {
  it('streams every pod in the namespace, with a filter and a stop button', async () => {
    streamWorkloadLogs.mockImplementation(openStream([
      '2026-05-20T10:00:00Z [default/a/app] hello from a',
      '2026-05-20T10:00:01Z [default/b/app] error from b',
    ]));
    const onStop = vi.fn();
    render(<ThemeProvider theme={theme}><NamespaceTail namespace="default" onStop={onStop} /></ThemeProvider>);

    expect(streamWorkloadLogs.mock.calls[0][0]).toEqual({ namespace: 'default' });
    await waitFor(() => expect(screen.getByText(/hello from a/)).toBeInTheDocument());
    expect(screen.getByText(/error from b/)).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText('Filter…'), { target: { value: 'error' } });
    expect(screen.queryByText(/hello from a/)).not.toBeInTheDocument();
    expect(screen.getByText(/error from b/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Stop' }));
    expect(onStop).toHaveBeenCalledOnce();
  });
});

describe('LogPanel — namespace page', () => {
  function NavigateTo({ to }: { to: string }) {
    const navigate = useNavigate();
    return <button onClick={() => navigate(to)}>go {to}</button>;
  }

  function renderAt(path: string) {
    return render(
      <MemoryRouter initialEntries={[path]}>
        <ThemeProvider theme={theme}>
          <Routes>
            <Route path="*" element={<><LogPanel /><NavigateTo to="/ns/kube-system" /></>} />
          </Routes>
        </ThemeProvider>
      </MemoryRouter>,
    );
  }

  it('offers a live tail of the namespace in the URL, and stops it on leaving the page', async () => {
    streamWorkloadLogs.mockImplementation(openStream([]));
    renderAt('/ns/monitoring');

    fireEvent.click(screen.getByRole('button', { name: 'Live tail every pod in monitoring' }));
    await waitFor(() => expect(streamWorkloadLogs).toHaveBeenCalledWith({ namespace: 'monitoring' }, expect.anything()));
    expect(screen.getByText('monitoring · all pods')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'go /ns/kube-system' }));
    expect(screen.queryByText('monitoring · all pods')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Live tail every pod in kube-system' })).toBeInTheDocument();
  });

  it('offers no namespace tail away from a namespace page', () => {
    renderAt('/ns/default/Deployment');
    expect(screen.queryByRole('button', { name: /Live tail every pod/ })).not.toBeInTheDocument();
  });
});
