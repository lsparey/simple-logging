import { render, screen, fireEvent, within } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { describe, expect, it, beforeEach, vi } from 'vitest';
import LogToolbar from './LogToolbar.js';
import { useLogStore } from '../../store/logStore.js';

vi.mock('./LogHistogram.js', () => ({
  default: () => <div data-testid="log-histogram">Histogram</div>,
}));

const theme = createTheme();
function Wrapper({ children }: { children: React.ReactNode }) {
  return <ThemeProvider theme={theme}>{children}</ThemeProvider>;
}

function renderToolbar(props: Partial<React.ComponentProps<typeof LogToolbar>> = {}) {
  return render(
    <LogToolbar
      namespace="default"
      kind="Pod"
      name="my-pod"
      liveEnabled={false}
      onLiveToggle={() => {}}
      searchMode="page"
      onSearchModeChange={() => {}}
      {...props}
    />,
    { wrapper: Wrapper },
  );
}

beforeEach(() => {
  useLogStore.setState({
    searchText: '',
    jsonLogging: false,
    jsonFormats: {},
    lines: [],
    selectedPodFilter: null,
    selectedContainerFilter: null,
    startTime: 0,
    endTime: 0,
  });
});

describe('LogToolbar — pod and container filters', () => {
  it('does not show either filter when all lines come from one pod/container', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/app] b',
      ],
    });
    renderToolbar();
    expect(screen.queryByLabelText('Filter by pod')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Filter by container')).not.toBeInTheDocument();
  });

  it('shows a container filter when lines span multiple containers, defaulting to all containers', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/sidecar] b',
      ],
    });
    renderToolbar();
    expect(screen.getByText('All containers')).toBeInTheDocument();
  });

  it('shows a pod filter when lines span multiple pods, defaulting to all pods', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/pod-a/app] a',
        '2026-05-20T10:00:01Z [default/pod-b/app] b',
      ],
    });
    renderToolbar({ kind: 'Deployment', name: 'web-app' });
    expect(screen.getByText('All pods')).toBeInTheDocument();
  });

  it('selecting a container updates the store', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/my-pod/app] a',
        '2026-05-20T10:00:01Z [default/my-pod/sidecar] b',
      ],
    });
    renderToolbar();

    fireEvent.mouseDown(screen.getByLabelText('Filter by container'));
    const listbox = within(screen.getByRole('listbox'));
    fireEvent.click(listbox.getByText('sidecar'));

    expect(useLogStore.getState().selectedContainerFilter).toBe('sidecar');
  });

  it('selecting a pod updates the store', () => {
    useLogStore.setState({
      lines: [
        '2026-05-20T10:00:00Z [default/pod-a/app] a',
        '2026-05-20T10:00:01Z [default/pod-b/app] b',
      ],
    });
    renderToolbar({ kind: 'Deployment', name: 'web-app' });

    fireEvent.mouseDown(screen.getByLabelText('Filter by pod'));
    const listbox = within(screen.getByRole('listbox'));
    fireEvent.click(listbox.getByText('pod-b'));

    expect(useLogStore.getState().selectedPodFilter).toBe('pod-b');
  });
});

describe('LogToolbar — search mode toggle', () => {
  it('shows the page-search text field in page mode', () => {
    renderToolbar({ searchMode: 'page' });
    expect(screen.getByPlaceholderText('Search…')).toBeInTheDocument();
  });

  it('hides the page-search text field in server mode', () => {
    renderToolbar({ searchMode: 'server' });
    expect(screen.queryByPlaceholderText('Search…')).not.toBeInTheDocument();
  });

  it('clicking the Server toggle notifies the parent', () => {
    const onSearchModeChange = vi.fn();
    renderToolbar({ searchMode: 'page', onSearchModeChange });
    fireEvent.click(screen.getByText('Server'));
    expect(onSearchModeChange).toHaveBeenCalledWith('server');
  });
});

describe('LogToolbar — download button', () => {
  it('downloads by kind and name when no pod filter is selected', () => {
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
    renderToolbar({ namespace: 'default', kind: 'Deployment', name: 'web-app' });

    fireEvent.click(screen.getByLabelText('Download logs'));

    expect(openSpy).toHaveBeenCalledTimes(1);
    const url = openSpy.mock.calls[0][0] as string;
    expect(url).toContain('/download?');
    expect(url).toContain('ns=default');
    expect(url).toContain('kind=Deployment');
    expect(url).toContain('name=web-app');
    expect(url).not.toContain('pod=');
    openSpy.mockRestore();
  });

  it('downloads by pod when a pod filter is selected, honoring the container filter and time range', () => {
    useLogStore.setState({ selectedPodFilter: 'web-app-abc', selectedContainerFilter: 'sidecar', startTime: 1000, endTime: 2000 });
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
    renderToolbar({ namespace: 'default', kind: 'Deployment', name: 'web-app' });

    fireEvent.click(screen.getByLabelText('Download logs'));

    const url = openSpy.mock.calls[0][0] as string;
    expect(url).toContain('pod=web-app-abc');
    expect(url).not.toContain('kind=');
    expect(url).toContain('container=sidecar');
    expect(url).toContain('from=1000');
    expect(url).toContain('to=2000');
    openSpy.mockRestore();
  });
});
