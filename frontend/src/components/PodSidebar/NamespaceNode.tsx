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
import { usePodList } from '../../hooks/usePodList.js';
import { useDeploymentList } from '../../hooks/useDeploymentList.js';
import { useLogStore } from '../../store/logStore.js';
import PodNode from './PodNode.js';
import DeploymentNode from './DeploymentNode.js';

interface Props {
  namespace: string;
  viewMode: 'pods' | 'deployments';
}

export default function NamespaceNode({ namespace, viewMode }: Props) {
  const selectedNamespace = useLogStore((s) => s.selectedNamespace);
  const [open, setOpen] = useState(() => selectedNamespace === namespace);
  const { pods } = usePodList(open && viewMode === 'pods' ? namespace : null);
  const { deployments } = useDeploymentList(open && viewMode === 'deployments' ? namespace : null);
  const navigate = useNavigate();

  function handleClick() {
    const next = !open;
    setOpen(next);
    if (next) {
      navigate(`/${viewMode === 'pods' ? 'pod' : 'deployment'}/${encodeURIComponent(namespace)}`);
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
          {viewMode === 'pods'
            ? pods.map((p) => <PodNode key={p.name} pod={p} />)
            : deployments.map((d) => <DeploymentNode key={d.name} deployment={d} />)}
        </List>
      </Collapse>
    </>
  );
}
