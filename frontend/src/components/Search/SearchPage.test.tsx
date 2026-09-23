import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import SearchPage from './SearchPage.js';
import { logClient } from '../../grpc/client.js';
import { useLogStore } from '../../store/logStore.js';

vi.mock('../../grpc/client.js', () => ({
  logClient: {
    searchLogs: vi.fn(),
    listPods: vi.fn(),
  },
}));

const searchLogs = vi.mocked(logClient.searchLogs);
const listPods = vi.mocked(logClient.listPods);

const theme = createTheme();
function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter>
      <ThemeProvider theme={theme}>{children}</ThemeProvider>
    </MemoryRouter>
  );
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
  listPods.mockReset();
  listPods.mockResolvedValue({ pods: [] });
  useLogStore.setState({
    selectedNamespace: null,
    selectedWorkloadKind: null,
    selectedWorkloadName: null,
    selectedContainerFilter: null,
    startTime: 0,
    endTime: 0,
  });
});

describe('SearchPage', () => {
  it('disables the Search button until a query is entered', () => {
    searchLogs.mockReturnValue(asyncIterableOf([]) as ReturnType<typeof logClient.searchLogs>);
    render(<SearchPage />, { wrapper: Wrapper });
    expect(screen.getByRole('button', { name: 'Search' })).toBeDisabled();
  });

  it('searches everywhere by default when namespace/kind/name are left blank', async () => {
    searchLogs.mockReturnValue(asyncIterableOf([]) as ReturnType<typeof logClient.searchLogs>);
    render(<SearchPage />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Query…'), { target: { value: 'error' } });
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => expect(searchLogs).toHaveBeenCalledTimes(1));
    expect(searchLogs.mock.calls[0][0]).toMatchObject({
      namespace: '',
      workloadKind: '',
      workloadName: '',
      query: 'error',
      regex: false,
      newestFirst: true,
    });
  });

  it('scopes the search to the entered namespace/kind/name', async () => {
    searchLogs.mockReturnValue(asyncIterableOf([]) as ReturnType<typeof logClient.searchLogs>);
    render(<SearchPage />, { wrapper: Wrapper });

    fireEvent.change(screen.getByLabelText('Namespace'), { target: { value: 'default' } });
    fireEvent.change(screen.getByLabelText('Workload kind'), { target: { value: 'Deployment' } });
    fireEvent.change(screen.getByLabelText('Workload name'), { target: { value: 'web-app' } });
    fireEvent.change(screen.getByPlaceholderText('Query…'), { target: { value: 'error' } });
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => expect(searchLogs).toHaveBeenCalledTimes(1));
    expect(searchLogs.mock.calls[0][0]).toMatchObject({
      namespace: 'default',
      workloadKind: 'Deployment',
      workloadName: 'web-app',
    });
  });

  it('renders streamed results with a namespace/pod/container chip and a truncated notice', async () => {
    searchLogs.mockReturnValue(
      asyncIterableOf([
        { line: '2026-05-20T10:00:00Z [default/web-app-abc/app] boom happened', namespace: 'default', pod: 'web-app-abc', container: 'app', truncated: false },
        { line: '', namespace: '', pod: '', container: '', truncated: true },
      ]) as ReturnType<typeof logClient.searchLogs>,
    );
    render(<SearchPage />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Query…'), { target: { value: 'boom' } });
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => {
      expect(screen.getByText((_, node) => node?.textContent === 'boom happened')).toBeInTheDocument();
    });
    expect(screen.getByText('default/web-app-abc/app')).toBeInTheDocument();
    expect(screen.getByText(/truncated/i)).toBeInTheDocument();
  });

  it('clicking a result selects the hit pod directly by kind Pod and navigates there', async () => {
    searchLogs.mockReturnValue(
      asyncIterableOf([
        { line: '2026-05-20T10:00:00Z [default/web-app-abc/app] boom happened', namespace: 'default', pod: 'web-app-abc', container: 'app', truncated: false },
      ]) as ReturnType<typeof logClient.searchLogs>,
    );
    listPods.mockResolvedValue({ pods: [{ name: 'web-app-abc', namespace: 'default', active: true, jsonLogging: true, containers: ['app'] }] });

    render(<SearchPage />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Query…'), { target: { value: 'boom' } });
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    const result = await screen.findByText((_, node) => node?.textContent === 'boom happened');
    fireEvent.click(result);

    await waitFor(() => {
      const s = useLogStore.getState();
      expect(s.selectedWorkloadKind).toBe('Pod');
      expect(s.selectedWorkloadName).toBe('web-app-abc');
      expect(s.selectedContainerFilter).toBe('app');
    });
  });

  it('shows an error message when the search fails', async () => {
    searchLogs.mockImplementation(() => {
      throw new Error('invalid regex');
    });
    render(<SearchPage />, { wrapper: Wrapper });

    fireEvent.change(screen.getByPlaceholderText('Query…'), { target: { value: '(' } });
    fireEvent.click(screen.getByLabelText('Regex'));
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => expect(screen.getByText(/invalid regex/)).toBeInTheDocument());
  });
});
