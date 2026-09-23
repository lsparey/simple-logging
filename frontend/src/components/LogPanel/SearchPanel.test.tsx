import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import SearchPanel from './SearchPanel.js';
import { logClient } from '../../grpc/client.js';

vi.mock('../../grpc/client.js', () => ({
  logClient: {
    searchLogs: vi.fn(),
    listWorkloads: vi.fn(),
  },
}));

const searchLogs = vi.mocked(logClient.searchLogs);

const theme = createTheme();
function Wrapper({ children }: { children: React.ReactNode }) {
  return <ThemeProvider theme={theme}>{children}</ThemeProvider>;
}

function asyncIterableOf<T>(items: T[]) {
  return {
    [Symbol.asyncIterator]: async function* () {
      for (const item of items) yield item;
    },
  };
}

beforeEach(() => {
  searchLogs.mockReset();
});

describe('SearchPanel', () => {
  it('disables the Search button until a query is entered', () => {
    searchLogs.mockReturnValue(asyncIterableOf([]) as ReturnType<typeof logClient.searchLogs>);
    render(<SearchPanel namespace="default" kind="Deployment" name="web-app" onJumpToContext={() => {}} />, { wrapper: Wrapper });
    expect(screen.getByRole('button', { name: 'Search' })).toBeDisabled();
  });

  it('searches scoped to the current workload by default', async () => {
    searchLogs.mockReturnValue(asyncIterableOf([]) as ReturnType<typeof logClient.searchLogs>);
    render(<SearchPanel namespace="default" kind="Deployment" name="web-app" onJumpToContext={() => {}} />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Search server-side…'), { target: { value: 'error' } });
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => expect(searchLogs).toHaveBeenCalledTimes(1));
    expect(searchLogs.mock.calls[0][0]).toMatchObject({
      namespace: 'default',
      workloadKind: 'Deployment',
      workloadName: 'web-app',
      query: 'error',
      regex: false,
      newestFirst: true,
    });
  });

  it('searches everywhere when the Everywhere checkbox is checked', async () => {
    searchLogs.mockReturnValue(asyncIterableOf([]) as ReturnType<typeof logClient.searchLogs>);
    render(<SearchPanel namespace="default" kind="Deployment" name="web-app" onJumpToContext={() => {}} />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Search server-side…'), { target: { value: 'error' } });
    fireEvent.click(screen.getByLabelText('Everywhere'));
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => expect(searchLogs).toHaveBeenCalledTimes(1));
    expect(searchLogs.mock.calls[0][0]).toMatchObject({
      namespace: '',
      workloadKind: '',
      workloadName: '',
    });
  });

  it('renders streamed results with pod/container and a truncated notice', async () => {
    searchLogs.mockReturnValue(
      asyncIterableOf([
        { line: '2026-05-20T10:00:00Z [default/web-app-abc/app] boom happened', namespace: 'default', pod: 'web-app-abc', container: 'app', truncated: false },
        { line: '', namespace: '', pod: '', container: '', truncated: true },
      ]) as ReturnType<typeof logClient.searchLogs>,
    );
    render(<SearchPanel namespace="default" kind="Deployment" name="web-app" onJumpToContext={() => {}} />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Search server-side…'), { target: { value: 'boom' } });
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => {
      expect(screen.getByText((_, node) => node?.textContent === 'boom happened')).toBeInTheDocument();
    });
    expect(screen.getByText('web-app-abc/app')).toBeInTheDocument();
    expect(screen.getByText(/truncated/i)).toBeInTheDocument();
  });

  it('calls onJumpToContext with the parsed timestamp when a result is clicked', async () => {
    searchLogs.mockReturnValue(
      asyncIterableOf([
        { line: '2026-05-20T10:00:00Z [default/web-app-abc/app] boom happened', namespace: 'default', pod: 'web-app-abc', container: 'app', truncated: false },
      ]) as ReturnType<typeof logClient.searchLogs>,
    );
    const onJumpToContext = vi.fn();
    render(<SearchPanel namespace="default" kind="Deployment" name="web-app" onJumpToContext={onJumpToContext} />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Search server-side…'), { target: { value: 'boom' } });
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    const result = await screen.findByText((_, node) => node?.textContent === 'boom happened');
    fireEvent.click(result);

    expect(onJumpToContext).toHaveBeenCalledWith({
      namespace: 'default',
      pod: 'web-app-abc',
      container: 'app',
      tsMs: Date.parse('2026-05-20T10:00:00Z'),
    });
  });

  it('shows an error message when the search fails', async () => {
    searchLogs.mockImplementation(() => {
      throw new Error('invalid regex');
    });
    render(<SearchPanel namespace="default" kind="Deployment" name="web-app" onJumpToContext={() => {}} />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Search server-side…'), { target: { value: '(' } });
    fireEvent.click(screen.getByLabelText('Regex'));
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => expect(screen.getByText(/invalid regex/)).toBeInTheDocument());
  });
});
