import { render, screen } from '@testing-library/react';
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import { MemoryRouter } from 'react-router-dom';
import AppShell from './AppShell.js';
import { useLogStore } from '../../store/logStore.js';

vi.mock('../PodSidebar/PodSidebar.js', () => ({
  default: () => <div>pod sidebar</div>,
}));
vi.mock('../PodSidebar/MobileSidebarNav.js', () => ({
  default: () => <div>mobile sidebar nav</div>,
  MOBILE_NAV_HEIGHT: 56,
}));
vi.mock('../LogPanel/LogPanel.js', () => ({
  default: () => <div>log panel</div>,
}));
vi.mock('../LogPanel/IndexPanel.js', () => ({
  default: () => <div>index panel</div>,
}));
vi.mock('../DataDashboard/DataDashboard.js', () => ({
  default: () => <div>data dashboard</div>,
}));

const theme = createTheme();

function renderAppShell() {
  return render(
    <MemoryRouter>
      <ThemeProvider theme={theme}>
        <AppShell />
      </ThemeProvider>
    </MemoryRouter>,
  );
}

/** Mocks window.matchMedia so MUI's useMediaQuery(theme.breakpoints.down('sm')) resolves to `matches`. */
function mockViewport(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
}

beforeEach(() => {
  useLogStore.setState({ selectedIndexKey: null, selectionKey: 0 });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('AppShell — sidebar layout', () => {
  it('shows the permanent side drawer on desktop widths, with no toggle button', () => {
    mockViewport(false);
    renderAppShell();
    expect(screen.getByText('pod sidebar')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Toggle sidebar' })).not.toBeInTheDocument();
  });

  it('shows the persistent bottom nav instead of the side drawer on mobile widths', () => {
    mockViewport(true);
    renderAppShell();
    expect(screen.getByText('mobile sidebar nav')).toBeInTheDocument();
    expect(screen.queryByText('pod sidebar')).not.toBeInTheDocument();
  });

  it('has no sidebar toggle button on mobile — the bottom nav is always visible', () => {
    mockViewport(true);
    renderAppShell();
    expect(screen.queryByRole('button', { name: 'Toggle sidebar' })).not.toBeInTheDocument();
  });
});
