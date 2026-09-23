import DnsIcon from '@mui/icons-material/Dns';
import LayersIcon from '@mui/icons-material/Layers';
import StorageIcon from '@mui/icons-material/Storage';
import DeviceHubIcon from '@mui/icons-material/DeviceHub';
import BoltIcon from '@mui/icons-material/Bolt';
import ScheduleIcon from '@mui/icons-material/Schedule';
import type { SvgIconComponent } from '@mui/icons-material';

export type WorkloadKind = 'Deployment' | 'StatefulSet' | 'DaemonSet' | 'Job' | 'CronJob' | 'Pod';

/**
 * The subset of WorkloadInfo (or a synthesized per-pod equivalent, for the
 * Pods section) that the sidebar's NamespaceNode/KindNode/WorkloadNode need
 * to render a row and select it.
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

export type TopLevelArea = 'indexes' | 'workloads';

/** Which of the mobile bottom nav's two icons (Indexes / Workloads) a path belongs to. */
export function inferAreaFromPath(pathname: string): TopLevelArea | null {
  if (pathname.startsWith('/index/') || pathname.startsWith('/indexes')) return 'indexes';
  if (pathname.startsWith('/ns/')) return 'workloads';
  return null;
}
