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
vi.mock('../../hooks/useWorkloadsByNamespace.js', () => ({
  useWorkloadsByNamespace: vi.fn(),
}));
vi.mock('../../hooks/usePodsByNamespace.js', () => ({
  usePodsByNamespace: vi.fn(),
}));

import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useWorkloadsByNamespace } from '../../hooks/useWorkloadsByNamespace.js';
import { usePodsByNamespace } from '../../hooks/usePodsByNamespace.js';

const mockUseNamespaces = vi.mocked(useNamespaces);
const mockUseWorkloadsByNamespace = vi.mocked(useWorkloadsByNamespace);
const mockUsePodsByNamespace = vi.mocked(usePodsByNamespace);

// Minimal MUI theme wrapper so MUI components render without warnings
const theme = createTheme();
function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter>
      <ThemeProvider theme={theme}>{children}</ThemeProvider>
    </MemoryRouter>
  );
}

const WEB_APP = { kind: 'Deployment', name: 'web-app', namespace: 'default', active: true, jsonLogging: false, pods: ['web-app-abc'] };
const COREDNS = { kind: 'Deployment', name: 'coredns', namespace: 'kube-system', active: true, jsonLogging: false, pods: ['coredns-abc'] };

// ---------------------------------------------------------------------------
// Shared setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks();
  // Default: every namespace has at least one matching workload, so section
  // menu / namespace-list tests don't need to think about the empty state.
  mockUseWorkloadsByNamespace.mockReturnValue({
    workloadsByNamespace: { default: [WEB_APP], 'kube-system': [COREDNS] },
    loading: false,
    error: null,
  });
  mockUsePodsByNamespace.mockReturnValue({ podsByNamespace: {}, loading: false, error: null });

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

  it('hides namespaces that have nothing of the chosen kind', () => {
    mockUseNamespaces.mockReturnValue({
      namespaces: ['default', 'kube-system'],
      loading: false,
      error: null,
    });
    mockUseWorkloadsByNamespace.mockReturnValue({
      workloadsByNamespace: { default: [WEB_APP], 'kube-system': [] },
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('Deployments'));
    expect(screen.getByText('default')).toBeInTheDocument();
    expect(screen.queryByText('kube-system')).not.toBeInTheDocument();
  });

  it('shows "No <kind>" when nothing has a workload of the chosen kind', () => {
    mockUseNamespaces.mockReturnValue({
      namespaces: ['default', 'kube-system'],
      loading: false,
      error: null,
    });
    mockUseWorkloadsByNamespace.mockReturnValue({
      workloadsByNamespace: { default: [], 'kube-system': [] },
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('CronJobs'));
    expect(screen.getByText('No CronJobs')).toBeInTheDocument();
    expect(screen.queryByText('default')).not.toBeInTheDocument();
  });

  it('the Pods section lists every pod, including ones owned by a Deployment', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    mockUsePodsByNamespace.mockReturnValue({
      podsByNamespace: {
        default: [
          { name: 'web-app-abc', namespace: 'default', active: true, jsonLogging: false, containers: ['app'] },
          { name: 'standalone-pod', namespace: 'default', active: true, jsonLogging: false, containers: ['app'] },
        ],
      },
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('Pods'));
    fireEvent.click(screen.getByText('default'));

    // web-app-abc is owned by the web-app Deployment (and so also appears
    // under Deployments), but Pods lists it individually regardless.
    await waitFor(() => {
      expect(screen.getByText('web-app-abc')).toBeInTheDocument();
      expect(screen.getByText('standalone-pod')).toBeInTheDocument();
    });
  });

  it('selecting a pod under Pods selects it by kind Pod, not its owner', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    mockUsePodsByNamespace.mockReturnValue({
      podsByNamespace: {
        default: [{ name: 'web-app-abc', namespace: 'default', active: true, jsonLogging: false, containers: ['app'] }],
      },
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('Pods'));
    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('web-app-abc')).toBeInTheDocument());

    act(() => fireEvent.click(screen.getByText('web-app-abc')));

    const s = useLogStore.getState();
    expect(s.selectedWorkloadKind).toBe('Pod');
    expect(s.selectedWorkloadName).toBe('web-app-abc');
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
});

// ---------------------------------------------------------------------------
// NamespaceNode — expand / collapse
// ---------------------------------------------------------------------------

describe('NamespaceNode', () => {
  it('renders the namespace label', () => {
    render(<NamespaceNode namespace="default" viewMode="Deployment" workloads={[]} />, { wrapper: Wrapper });
    expect(screen.getByText('default')).toBeInTheDocument();
  });

  it('does not show children before the node is expanded', () => {
    render(<NamespaceNode namespace="default" viewMode="Deployment" workloads={[WEB_APP]} />, { wrapper: Wrapper });
    expect(screen.queryByText('web-app')).not.toBeInTheDocument();
  });

  it('shows the given workloads after expanding the namespace', async () => {
    render(
      <NamespaceNode
        namespace="default"
        viewMode="Deployment"
        workloads={[WEB_APP, { ...WEB_APP, name: 'api-server' }]}
      />,
      { wrapper: Wrapper },
    );

    fireEvent.click(screen.getByText('default'));

    await waitFor(() => {
      expect(screen.getByText('web-app')).toBeInTheDocument();
      expect(screen.getByText('api-server')).toBeInTheDocument();
    });
  });

  it('shows a pod-count badge only for workloads with more than one pod', async () => {
    render(
      <NamespaceNode
        namespace="default"
        viewMode="Pod"
        workloads={[
          { kind: 'Pod', name: 'single-pod-workload', namespace: 'default', active: true, jsonLogging: false, pods: ['single-pod-workload'] },
          { kind: 'Pod', name: 'other-pod', namespace: 'default', active: true, jsonLogging: false, pods: ['other-pod'] },
        ]}
      />,
      { wrapper: Wrapper },
    );
    fireEvent.click(screen.getByText('default'));

    await waitFor(() => {
      expect(screen.getByText('single-pod-workload')).toBeInTheDocument();
      expect(screen.getByText('other-pod')).toBeInTheDocument();
    });
    expect(screen.queryByText('2')).not.toBeInTheDocument();
  });

  it('collapses back after a second click', async () => {
    render(<NamespaceNode namespace="default" viewMode="Deployment" workloads={[WEB_APP]} />, { wrapper: Wrapper });

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
    render(<NamespaceNode namespace="default" viewMode="Deployment" workloads={[WEB_APP]} />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('default'));

    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());

    act(() => fireEvent.click(screen.getByText('web-app')));

    const s = useLogStore.getState();
    expect(s.selectedNamespace).toBe('default');
    expect(s.selectedWorkloadKind).toBe('Deployment');
    expect(s.selectedWorkloadName).toBe('web-app');
  });
});
