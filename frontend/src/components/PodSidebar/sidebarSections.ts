import DnsIcon from '@mui/icons-material/Dns';
import LayersIcon from '@mui/icons-material/Layers';
import KeyIcon from '@mui/icons-material/Key';
import type { SvgIconComponent } from '@mui/icons-material';

export type SidebarSection = 'pods' | 'deployments' | 'indexes';

export const SIDEBAR_SECTIONS: { key: SidebarSection; label: string; Icon: SvgIconComponent }[] = [
  { key: 'pods', label: 'Pods', Icon: DnsIcon },
  { key: 'deployments', label: 'Deployments', Icon: LayersIcon },
  { key: 'indexes', label: 'Indexes', Icon: KeyIcon },
];

export function inferSectionFromPath(pathname: string): SidebarSection | null {
  if (pathname.startsWith('/index/') || pathname.startsWith('/indexes')) return 'indexes';
  if (pathname.startsWith('/pod/')) return 'pods';
  if (pathname.startsWith('/deployment/')) return 'deployments';
  return null;
}
