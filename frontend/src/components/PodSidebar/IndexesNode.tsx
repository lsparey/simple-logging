import { useState } from 'react';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Collapse from '@mui/material/Collapse';
import ExpandLess from '@mui/icons-material/ExpandLess';
import ExpandMore from '@mui/icons-material/ExpandMore';
import KeyIcon from '@mui/icons-material/Key';
import { useNavigate } from 'react-router-dom';
import { useLogStore } from '../../store/logStore.js';
import IndexSidebar from './IndexSidebar.js';

interface Props {
  onLeafSelect?: () => void;
}

/**
 * Top-level "Indexes" tree row, sitting alongside the namespace folders.
 * Expanding it reveals the index list inline (via IndexSidebar) rather than
 * replacing the rest of the tree. Unlike a namespace or kind row, opening it
 * also switches the main panel to IndexPanel (enterIndexMode) — indexes
 * aren't a pure grouping mechanism the way namespaces/kinds are, and
 * IndexPanel's own "Create Index" button is the only way to create one when
 * none exist yet, so it needs to be reachable without first having a key to
 * click.
 */
export default function IndexesNode({ onLeafSelect }: Props) {
  const selectedIndexKey = useLogStore((s) => s.selectedIndexKey);
  const enterIndexMode = useLogStore((s) => s.enterIndexMode);
  const [open, setOpen] = useState(() => selectedIndexKey !== null);
  const navigate = useNavigate();

  function handleClick() {
    const next = !open;
    setOpen(next);
    if (next) {
      enterIndexMode();
      navigate('/indexes');
    }
  }

  return (
    <List disablePadding>
      <ListItem disablePadding>
        <ListItemButton onClick={handleClick} dense>
          <ListItemIcon sx={{ minWidth: 32 }}>
            <KeyIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText primary="Indexes" slotProps={{ primary: { variant: 'body2' } }} />
          {open ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
        </ListItemButton>
      </ListItem>
      <Collapse in={open} unmountOnExit>
        <IndexSidebar onLeafSelect={onLeafSelect} />
      </Collapse>
    </List>
  );
}
