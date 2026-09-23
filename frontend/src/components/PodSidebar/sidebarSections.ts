import DnsIcon from '@mui/icons-material/Dns';
import LayersIcon from '@mui/icons-material/Layers';
import StorageIcon from '@mui/icons-material/Storage';
import DeviceHubIcon from '@mui/icons-material/DeviceHub';
import BoltIcon from '@mui/icons-material/Bolt';
import ScheduleIcon from '@mui/icons-material/Schedule';
import KeyIcon from '@mui/icons-material/Key';
import type { SvgIconComponent } from '@mui/icons-material';

export type WorkloadKind = 'Deployment' | 'StatefulSet' | 'DaemonSet' | 'Job' | 'CronJob' | 'Pod';

/**
 * The subset of WorkloadInfo (or a synthesized per-pod equivalent, for the
 * Pods section) that the sidebar's NamespaceNode/WorkloadNode need to render
 * a row and select it.
 */
export interface SidebarWorkload {
  kind: string;
  name: string;
  namespace: string;
  active: boolean;
  jsonLogging: boolean;
  pods: string[];
}

export const WORKLOAD_KIND_SECTIONS: { key: WorkloadKind; label: string; Icon: SvgIconComponent }[] = [
  { key: 'Deployment', label: 'Deployments', Icon: LayersIcon },
  { key: 'StatefulSet', label: 'StatefulSets', Icon: StorageIcon },
  { key: 'DaemonSet', label: 'DaemonSets', Icon: DeviceHubIcon },
  { key: 'Job', label: 'Jobs', Icon: BoltIcon },
  { key: 'CronJob', label: 'CronJobs', Icon: ScheduleIcon },
  { key: 'Pod', label: 'Pods', Icon: DnsIcon },
];

export type SidebarSection = 'indexes' | WorkloadKind;

export const SIDEBAR_SECTIONS: { key: SidebarSection; label: string; Icon: SvgIconComponent }[] = [
  { key: 'indexes', label: 'Indexes', Icon: KeyIcon },
  ...WORKLOAD_KIND_SECTIONS,
];

const WORKLOAD_KIND_KEYS = new Set<string>(WORKLOAD_KIND_SECTIONS.map((s) => s.key));

export function inferSectionFromPath(pathname: string): SidebarSection | null {
  if (pathname.startsWith('/index/') || pathname.startsWith('/indexes')) return 'indexes';
  const m = pathname.match(/^\/ns\/[^/]+\/([^/]+)/);
  if (m && WORKLOAD_KIND_KEYS.has(m[1])) return m[1] as WorkloadKind;
  return null;
}
