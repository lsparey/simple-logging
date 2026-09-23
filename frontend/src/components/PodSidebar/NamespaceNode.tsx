import { useState } from 'react';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Collapse from '@mui/material/Collapse';
import ExpandLess from '@mui/icons-material/ExpandLess';
import ExpandMore from '@mui/icons-material/ExpandMore';
import FolderIcon from '@mui/icons-material/Folder';
import List from '@mui/material/List';
import { useNavigate } from 'react-router-dom';
import { useLogStore } from '../../store/logStore.js';
import WorkloadNode from './WorkloadNode.js';
import type { SidebarWorkload, WorkloadKind } from './sidebarSections.js';

interface Props {
  namespace: string;
  viewMode: WorkloadKind;
  workloads: SidebarWorkload[];
  onLeafSelect?: () => void;
}

export default function NamespaceNode({ namespace, viewMode, workloads, onLeafSelect }: Props) {
  const selectedNamespace = useLogStore((s) => s.selectedNamespace);
  const [open, setOpen] = useState(() => selectedNamespace === namespace);
  const navigate = useNavigate();

  function handleClick() {
    const next = !open;
    setOpen(next);
    if (next) {
      navigate(`/ns/${encodeURIComponent(namespace)}/${encodeURIComponent(viewMode)}`);
    }
  }

  return (
    <>
      <ListItem disablePadding>
        <ListItemButton onClick={handleClick} dense>
          <ListItemIcon sx={{ minWidth: 32 }}>
            <FolderIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText
            primary={namespace}
            slotProps={{ primary: { variant: 'body2' } }}
          />
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
