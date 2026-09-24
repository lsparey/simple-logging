import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach } from 'vitest';
import { act } from 'react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import NamespaceNode from './NamespaceNode.js';
import KindNode from './KindNode.js';
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
vi.mock('../../hooks/useIndexList.js', () => ({
  useIndexList: vi.fn(),
}));

import { useNamespaces } from '../../hooks/useNamespaces.js';
import { useWorkloadsByNamespace } from '../../hooks/useWorkloadsByNamespace.js';
import { usePodsByNamespace } from '../../hooks/usePodsByNamespace.js';
import { useIndexList } from '../../hooks/useIndexList.js';

const mockUseNamespaces = vi.mocked(useNamespaces);
const mockUseWorkloadsByNamespace = vi.mocked(useWorkloadsByNamespace);
const mockUsePodsByNamespace = vi.mocked(usePodsByNamespace);
const mockUseIndexList = vi.mocked(useIndexList);

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
const CACHE = { kind: 'StatefulSet', name: 'cache', namespace: 'default', active: true, jsonLogging: false, pods: ['cache-0'] };
const COREDNS = { kind: 'Deployment', name: 'coredns', namespace: 'kube-system', active: true, jsonLogging: false, pods: ['coredns-abc'] };

// ---------------------------------------------------------------------------
// Shared setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.clearAllMocks();
  mockUseWorkloadsByNamespace.mockReturnValue({
    workloadsByNamespace: { default: [WEB_APP, CACHE], 'kube-system': [COREDNS] },
    loading: false,
    error: null,
  });
  mockUsePodsByNamespace.mockReturnValue({ podsByNamespace: {}, loading: false, error: null });
  mockUseIndexList.mockReturnValue({ indexes: [{ key: 'companyUuid' }], loading: false, error: null, reload: vi.fn() });

  useLogStore.setState({
    selectedNamespace: null,
    selectedWorkloadKind: null,
    selectedWorkloadName: null,
    selectedIndexKey: null,
    lines: [],
    searchText: '',
    prevPageToken: '',
    nextPageToken: '',
    mode: 'idle',
  });
});

// ---------------------------------------------------------------------------
// PodSidebar — the namespace -> kind -> workload accordion
// ---------------------------------------------------------------------------

describe('PodSidebar', () => {
  it('shows Indexes and every namespace with at least one workload', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default', 'kube-system'], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });
    expect(screen.getByText('Indexes')).toBeInTheDocument();
    expect(screen.getByText('default')).toBeInTheDocument();
    expect(screen.getByText('kube-system')).toBeInTheDocument();
  });

  it('lists the default namespace first, ahead of alphabetically earlier ones', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['cert-manager', 'default', 'kube-system'], loading: false, error: null });
    mockUseWorkloadsByNamespace.mockReturnValue({
      workloadsByNamespace: { 'cert-manager': [{ ...COREDNS, namespace: 'cert-manager' }], default: [WEB_APP], 'kube-system': [COREDNS] },
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });
    const labels = ['cert-manager', 'default', 'kube-system'].map((ns) => screen.getByText(ns));
    expect(labels[1].compareDocumentPosition(labels[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(labels[0].compareDocumentPosition(labels[2]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('hides a namespace with no workloads at all', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default', 'empty-ns'], loading: false, error: null });
    mockUseWorkloadsByNamespace.mockReturnValue({
      workloadsByNamespace: { default: [WEB_APP], 'empty-ns': [] },
      loading: false,
      error: null,
    });
    render(<PodSidebar />, { wrapper: Wrapper });
    expect(screen.getByText('default')).toBeInTheDocument();
    expect(screen.queryByText('empty-ns')).not.toBeInTheDocument();
  });

  it('shows "No workloads" when nothing exists anywhere', () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    mockUseWorkloadsByNamespace.mockReturnValue({ workloadsByNamespace: { default: [] }, loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });
    expect(screen.getByText('No workloads')).toBeInTheDocument();
  });

  it('expanding a namespace shows only the workload kinds present in it', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('default'));

    await waitFor(() => {
      expect(screen.getByText('Deployments')).toBeInTheDocument();
      expect(screen.getByText('StatefulSets')).toBeInTheDocument();
    });
    expect(screen.queryByText('DaemonSets')).not.toBeInTheDocument();
  });

  it('expanding a kind shows its workloads', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('Deployments')).toBeInTheDocument());
    fireEvent.click(screen.getByText('Deployments'));

    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());
  });

  it('selecting a workload updates the store', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('Deployments')).toBeInTheDocument());
    fireEvent.click(screen.getByText('Deployments'));
    await waitFor(() => expect(screen.getByText('web-app')).toBeInTheDocument());

    act(() => fireEvent.click(screen.getByText('web-app')));

    const s = useLogStore.getState();
    expect(s.selectedNamespace).toBe('default');
    expect(s.selectedWorkloadKind).toBe('Deployment');
    expect(s.selectedWorkloadName).toBe('web-app');
  });

  it('the Pods kind lists every pod, including ones owned by a Deployment', async () => {
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

    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('Pods')).toBeInTheDocument());
    fireEvent.click(screen.getByText('Pods'));

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

    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('Pods')).toBeInTheDocument());
    fireEvent.click(screen.getByText('Pods'));
    await waitFor(() => expect(screen.getByText('web-app-abc')).toBeInTheDocument());

    act(() => fireEvent.click(screen.getByText('web-app-abc')));

    const s = useLogStore.getState();
    expect(s.selectedWorkloadKind).toBe('Pod');
    expect(s.selectedWorkloadName).toBe('web-app-abc');
  });

  it('expanding Indexes reveals index keys inline, alongside the namespace tree', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('Indexes'));

    // The namespace tree is still visible — Indexes expanded in place, not a full-screen swap.
    await waitFor(() => expect(screen.getByText('companyUuid')).toBeInTheDocument());
    expect(screen.getByText('default')).toBeInTheDocument();
  });

  it('collapsing Indexes hides its index keys again', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('Indexes'));
    await waitFor(() => expect(screen.getByText('companyUuid')).toBeInTheDocument());

    fireEvent.click(screen.getByText('Indexes'));
    await waitFor(() => expect(screen.queryByText('companyUuid')).not.toBeInTheDocument());
  });

  it('selecting an index key updates the store without collapsing the namespace tree', async () => {
    mockUseNamespaces.mockReturnValue({ namespaces: ['default'], loading: false, error: null });
    render(<PodSidebar />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('Indexes'));
    await waitFor(() => expect(screen.getByText('companyUuid')).toBeInTheDocument());

    act(() => fireEvent.click(screen.getByText('companyUuid')));

    expect(useLogStore.getState().selectedIndexKey).toBe('companyUuid');
    expect(screen.getByText('default')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// NamespaceNode — expand / collapse
// ---------------------------------------------------------------------------

describe('NamespaceNode', () => {
  it('renders the namespace label', () => {
    render(<NamespaceNode namespace="default" workloads={[]} />, { wrapper: Wrapper });
    expect(screen.getByText('default')).toBeInTheDocument();
  });

  it('does not show kind rows before the node is expanded', () => {
    render(<NamespaceNode namespace="default" workloads={[WEB_APP]} />, { wrapper: Wrapper });
    expect(screen.queryByText('Deployments')).not.toBeInTheDocument();
  });

  it('groups workloads by kind after expanding', async () => {
    render(<NamespaceNode namespace="default" workloads={[WEB_APP, CACHE]} />, { wrapper: Wrapper });
    fireEvent.click(screen.getByText('default'));
    await waitFor(() => {
      expect(screen.getByText('Deployments')).toBeInTheDocument();
      expect(screen.getByText('StatefulSets')).toBeInTheDocument();
    });
  });

  it('collapses back after a second click', async () => {
    render(<NamespaceNode namespace="default" workloads={[WEB_APP]} />, { wrapper: Wrapper });

    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.getByText('Deployments')).toBeInTheDocument());

    fireEvent.click(screen.getByText('default'));
    await waitFor(() => expect(screen.queryByText('Deployments')).not.toBeInTheDocument());
  });
});

// ---------------------------------------------------------------------------
// KindNode — expand / collapse and selection
// ---------------------------------------------------------------------------

describe('KindNode', () => {
  it('does not show workloads before expanding', () => {
    render(<KindNode namespace="default" kind="Deployment" workloads={[WEB_APP]} />, { wrapper: Wrapper });
    expect(screen.queryByText('web-app')).not.toBeInTheDocument();
  });

  it('shows the given workloads after expanding', async () => {
    render(
      <KindNode namespace="default" kind="Deployment" workloads={[WEB_APP, { ...WEB_APP, name: 'api-server' }]} />,
      { wrapper: Wrapper },
    );
    fireEvent.click(screen.getByText('Deployments'));
    await waitFor(() => {
      expect(screen.getByText('web-app')).toBeInTheDocument();
      expect(screen.getByText('api-server')).toBeInTheDocument();
    });
  });

  it('shows a pod-count badge only for workloads with more than one pod', async () => {
    render(
      <KindNode
        namespace="default"
        kind="Pod"
        workloads={[
          { kind: 'Pod', name: 'single-pod-workload', namespace: 'default', active: true, jsonLogging: false, pods: ['single-pod-workload'] },
          { kind: 'Pod', name: 'other-pod', namespace: 'default', active: true, jsonLogging: false, pods: ['other-pod'] },
        ]}
      />,
      { wrapper: Wrapper },
    );
    fireEvent.click(screen.getByText('Pods'));
    await waitFor(() => {
      expect(screen.getByText('single-pod-workload')).toBeInTheDocument();
      expect(screen.getByText('other-pod')).toBeInTheDocument();
    });
    expect(screen.queryByText('2')).not.toBeInTheDocument();
  });
});

describe('WorkloadNode — container badge', () => {
  it('shows how many containers a pod has when it has more than one', async () => {
    render(
      <KindNode
        namespace="default"
        kind="Pod"
        workloads={[
          { kind: 'Pod', name: 'with-sidecar', namespace: 'default', active: true, jsonLogging: false, pods: ['with-sidecar'], containers: ['app', 'istio-proxy'] },
          { kind: 'Pod', name: 'single', namespace: 'default', active: true, jsonLogging: false, pods: ['single'], containers: ['app'] },
        ]}
      />,
      { wrapper: Wrapper },
    );
    fireEvent.click(screen.getByText('Pods'));
    await waitFor(() => expect(screen.getByText('with-sidecar')).toBeInTheDocument());
    expect(screen.getByLabelText('2 containers')).toBeInTheDocument();
    expect(screen.queryByLabelText('1 containers')).not.toBeInTheDocument();
  });
});
