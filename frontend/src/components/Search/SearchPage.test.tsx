import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
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
    listNamespaces: vi.fn(),
    listWorkloads: vi.fn(),
  },
}));

const searchLogs = vi.mocked(logClient.searchLogs);
const listPods = vi.mocked(logClient.listPods);
const listNamespaces = vi.mocked(logClient.listNamespaces);
const listWorkloads = vi.mocked(logClient.listWorkloads);

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
  listNamespaces.mockReset();
  listNamespaces.mockResolvedValue({ namespaces: ['default', 'kube-system'] });
  listWorkloads.mockReset();
  listWorkloads.mockResolvedValue({ workloads: [] });
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

  it('shows "All" as the top option for namespace and workload kind', () => {
    render(<SearchPage />, { wrapper: Wrapper });

    fireEvent.mouseDown(screen.getByLabelText('Namespace'));
    const namespaceOptions = within(screen.getByRole('listbox')).getAllByRole('option');
    expect(namespaceOptions[0]).toHaveTextContent('All');
    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' });

    fireEvent.mouseDown(screen.getByLabelText('Workload kind'));
    const kindOptions = within(screen.getByRole('listbox')).getAllByRole('option');
    expect(kindOptions[0]).toHaveTextContent('All');
  });

  it('scopes the search to the selected namespace/kind and typed name', async () => {
    searchLogs.mockReturnValue(asyncIterableOf([]) as ReturnType<typeof logClient.searchLogs>);
    render(<SearchPage />, { wrapper: Wrapper });

    fireEvent.mouseDown(screen.getByLabelText('Namespace'));
    fireEvent.click(await within(screen.getByRole('listbox')).findByText('default'));

    fireEvent.mouseDown(screen.getByLabelText('Workload kind'));
    fireEvent.click(within(screen.getByRole('listbox')).getByText('Deployments'));

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

  it('suggests workload names for the selected namespace and kind', async () => {
    listWorkloads.mockResolvedValue({
      workloads: [
        { kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] },
        { kind: 'StatefulSet', name: 'cache', namespace: 'default', active: true, jsonLogging: false, pods: ['cache-0'] },
      ],
    });
    render(<SearchPage />, { wrapper: Wrapper });

    fireEvent.mouseDown(screen.getByLabelText('Namespace'));
    fireEvent.click(await within(screen.getByRole('listbox')).findByText('default'));
    fireEvent.mouseDown(screen.getByLabelText('Workload kind'));
    fireEvent.click(within(screen.getByRole('listbox')).getByText('Deployments'));

    fireEvent.mouseDown(screen.getByLabelText('Workload name'));
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'web-app' })).toBeInTheDocument();
    });
    // Filtered to Deployment kind, so the StatefulSet "cache" isn't offered.
    expect(screen.queryByRole('option', { name: 'cache' })).not.toBeInTheDocument();
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
