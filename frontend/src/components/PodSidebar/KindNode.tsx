import { useState } from 'react';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Collapse from '@mui/material/Collapse';
import ExpandLess from '@mui/icons-material/ExpandLess';
import ExpandMore from '@mui/icons-material/ExpandMore';
import List from '@mui/material/List';
import { useNavigate } from 'react-router-dom';
import { useLogStore } from '../../store/logStore.js';
import WorkloadNode from './WorkloadNode.js';
import { WORKLOAD_KIND_SECTIONS, type SidebarWorkload, type WorkloadKind } from './sidebarSections.js';

interface Props {
  namespace: string;
  kind: WorkloadKind;
  workloads: SidebarWorkload[];
  onLeafSelect?: () => void;
}

export default function KindNode({ namespace, kind, workloads, onLeafSelect }: Props) {
  const selectedNamespace = useLogStore((s) => s.selectedNamespace);
  const selectedWorkloadKind = useLogStore((s) => s.selectedWorkloadKind);
  const [open, setOpen] = useState(() => selectedNamespace === namespace && selectedWorkloadKind === kind);
  const navigate = useNavigate();
  const { label, Icon } = WORKLOAD_KIND_SECTIONS.find((s) => s.key === kind)!;

  function handleClick() {
    const next = !open;
    setOpen(next);
    if (next) {
      navigate(`/ns/${encodeURIComponent(namespace)}/${encodeURIComponent(kind)}`);
    }
  }

  return (
    <>
      <ListItem disablePadding sx={{ pl: 2 }}>
        <ListItemButton onClick={handleClick} dense>
          <ListItemIcon sx={{ minWidth: 28 }}>
            <Icon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary={label} slotProps={{ primary: { variant: 'body2' } }} />
          {open ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
        </ListItemButton>
      </ListItem>
      <Collapse in={open} unmountOnExit>
        <List disablePadding>
          {workloads.map((w) => (
            <WorkloadNode key={`${w.kind}/${w.name}`} workload={w} onSelect={onLeafSelect} />
          ))}
        </List>
      </Collapse>
    </>
  );
}
