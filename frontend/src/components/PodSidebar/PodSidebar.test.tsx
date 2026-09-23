import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { act } from 'react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import NamespaceNode from './NamespaceNode.js';
import PodSidebar from './PodSidebar.js';
import { useLogStore } from '../../store/logStore.js';

// ---------------------------------------------------------------------------
// Mock hooks that make gRPC calls
// ---------------------------------------------------------------------------

vi.mock('../../hooks/useNamespaces.js', () => ({
  useNamespaces: vi.fn(),
}));
vi.mock('../../hooks/useWorkloadList.js', () => ({
  useWorkloadList: vi.fn(),
}));

import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useWorkloadList } from '../../hooks/useWorkloadList.js';

const mockUseNamespaces = vi.mocked(useNamespaces);
const mockUseWorkloadList = vi.mocked(useWorkloadList);

// Minimal MUI theme wrapper so MUI components render without warnings
const theme = createTheme();
function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter>
      <ThemeProvider theme={theme}>{children}</ThemeProvider>
    </MemoryRouter>
  );
}

// ---------------------------------------------------------------------------
// Shared setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks();
  // Default: no workloads until the namespace is expanded
  mockUseWorkloadList.mockReturnValue({ workloads: [], loading: false, error: null });

  // Reset store selection state
  useLogStore.setState({
    selectedNamespace: null,
    selectedWorkloadKind: null,
    selectedWorkloadName: null,
    lines: [],
    searchText: '',
    prevPageToken: '',
    nextPageToken: '',
    mode: 'idle',
  });
});

// ---------------------------------------------------------------------------
// PodSidebar
// ---------------------------------------------------------------------------

describe('PodSidebar', () => {
  it('shows the section menu by default, with no namespaces until a section is chosen', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: [], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });
    expect(screen.getByText('Indexes')).toBeInTheDocument();
    expect(screen.getByText('Deployments')).toBeInTheDocument();
    expect(screen.getByText('StatefulSets')).toBeInTheDocument();
    expect(screen.getByText('DaemonSets')).toBeInTheDocument();
    expect(screen.getByText('Jobs')).toBeInTheDocument();
    expect(screen.getByText('CronJobs')).toBeInTheDocument();
    expect(screen.getByText('Pods')).toBeInTheDocument();
  });

  it('shows a loading spinner while namespaces are loading', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: [], loading: true, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('Deployments'));
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('shows an error message when namespace fetch fails', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: [], loading: false, error: 'connection refused' });
    render(<PodSidebar />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('Deployments'));
    expect(screen.getByText(/connection refused/)).toBeInTheDocument();
  });

  it('renders namespace buttons after choosing a workload kind', () => {
    mockUseNamespaces.mockReturnValue({
      namespaces: ['default', 'kube-system'],
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('Deployments'));
    expect(screen.getByText('default')).toBeInTheDocument();
    expect(screen.getByText('kube-system')).toBeInTheDocument();
  });

  it('shows a back button and the section label after choosing a section, and returns to the menu', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('Deployments'));
    expect(screen.getByText('default')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Back' }));
    expect(screen.queryByText('default')).not.toBeInTheDocument();
    expect(screen.getByText('Deployments')).toBeInTheDocument();
    expect(screen.getByText('Pods')).toBeInTheDocument();
  });

  it('switching kinds shows only that kind\'s workloads', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    mockUseWorkloadList.mockReturnValue({
      workloads: [
        { kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] },
        { kind: 'Pod', name: 'standalone', namespace: 'default', active: true, jsonLogging: false, pods: ['standalone'] },
      ],
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('Deployments'));
    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());
    expect(screen.queryByText('standalone')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// NamespaceNode — expand / collapse
// ---------------------------------------------------------------------------

describe('NamespaceNode', () => {
  it('renders the namespace label', () => {
    render(<NamespaceNode namespace="default" viewMode="Deployment" />, { wrapper: Wrapper });
    expect(screen.getByText('default')).toBeInTheDocument();
  });

  it('does not show children before the node is expanded', () => {
    mockUseWorkloadList.mockReturnValue({
      workloads: [{ kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] }],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="Deployment" />, { wrapper: Wrapper });
    expect(screen.queryByText('web-app')).not.toBeInTheDocument();
  });

  it('shows workloads of the matching kind after expanding the namespace', async () => {
    mockUseWorkloadList.mockReturnValue({
      workloads: [
        { kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] },
        { kind: 'Deployment', name: 'api-server', namespace: 'default', active: false, jsonLogging: false, pods: ['api-server-abc'] },
        { kind: 'StatefulSet', name: 'cache', namespace: 'default', active: true, jsonLogging: false, pods: ['cache-0'] },
      ],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="Deployment" />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('default'));

    await waitFor(() => {
      expect(screen.getByText('web-app')).toBeInTheDocument();
      expect(screen.getByText('api-server')).toBeInTheDocument();
    });
    expect(screen.queryByText('cache')).not.toBeInTheDocument();
  });

  it('shows a pod-count badge only for workloads with more than one pod', async () => {
    mockUseWorkloadList.mockReturnValue({
      workloads: [
        { kind: 'Pod', name: 'single-pod-workload', namespace: 'default', active: true, jsonLogging: false, pods: ['single-pod-workload'] },
        { kind: 'Pod', name: 'other-pod', namespace: 'default', active: true, jsonLogging: false, pods: ['other-pod'] },
      ],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="Pod" />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('default'));

    await waitFor(() => {
      expect(screen.getByText('single-pod-workload')).toBeInTheDocument();
      expect(screen.getByText('other-pod')).toBeInTheDocument();
    });
    expect(screen.queryByText('2')).not.toBeInTheDocument();
  });

  it('collapses back after a second click', async () => {
    mockUseWorkloadList.mockReturnValue({
      workloads: [{ kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] }],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="Deployment" />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('default')); // expand
    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());

    fireEvent.click(screen.getByText('default')); // collapse
    await waitFor(() => expect(screen.queryByText('web-app')).not.toBeInTheDocument());
  });
});

// ---------------------------------------------------------------------------
// WorkloadNode — selection updates the store
// ---------------------------------------------------------------------------

describe('NamespaceNode — workload selection', () => {
  it('selecting a workload updates the store', async () => {
    mockUseWorkloadList.mockReturnValue({
      workloads: [{ kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] }],
      loading: false,
      error: null,
    });
    render(<NamespaceNode namespace="default" viewMode="Deployment" />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('default'));

    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());

    act(() => fireEvent.click(screen.getByText('web-app')));

    const s = useLogStore.getState();
    expect(s.selectedNamespace).toBe('default');
    expect(s.selectedWorkloadKind).toBe('Deployment');
    expect(s.selectedWorkloadName).toBe('web-app');
  });
});
