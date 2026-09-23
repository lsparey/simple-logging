import LayersIcon from '@mui/icons-material/Layers';
import KeyIcon from '@mui/icons-material/Key';
import type { SvgIconComponent } from '@mui/icons-material';

export type SidebarSection = 'workloads' | 'indexes';

export const SIDEBAR_SECTIONS: { key: SidebarSection; label: string; Icon: SvgIconComponent }[] = [
  { key: 'workloads', label: 'Workloads', Icon: LayersIcon },
  { key: 'indexes', label: 'Indexes', Icon: KeyIcon },
];

export function inferSectionFromPath(pathname: string): SidebarSection | null {
  if (pathname.startsWith('/index/') || pathname.startsWith('/indexes')) return 'indexes';
  if (pathname.startsWith('/ns/')) return 'workloads';
  return null;
}
