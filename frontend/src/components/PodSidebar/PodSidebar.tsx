import Box from '@mui/material/Box';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import KeyIcon from '@mui/icons-material/Key';
import { useNavigate } from 'react-router-dom';
import { useLogStore } from '../../store/logStore.js';
import WorkloadTree from './WorkloadTree.js';
import IndexSidebar from './IndexSidebar.js';

export default function PodSidebar() {
  const navigate = useNavigate();
  const { enterIndexMode, leaveIndexMode } = useLogStore();
  const inIndexMode = useLogStore((s) => s.selectedIndexKey !== null);

  function openIndexes() {
    enterIndexMode();
    navigate('/indexes');
  }

  function backToTree() {
    leaveIndexMode();
    navigate('/');
  }

  if (inIndexMode) {
    return (
      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <List disablePadding>
          <ListItem disablePadding sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <ListItemButton onClick={backToTree} dense aria-label="Back">
              <ListItemIcon sx={{ minWidth: 32 }}>
                <ArrowBackIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText primary="Indexes" slotProps={{ primary: { variant: 'body2' } }} />
            </ListItemButton>
          </ListItem>
        </List>
        <IndexSidebar />
      </Box>
    );
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <List disablePadding>
        <ListItem disablePadding>
          <ListItemButton onClick={openIndexes} dense>
            <ListItemIcon sx={{ minWidth: 32 }}>
              <KeyIcon fontSize="small" />
            </ListItemIcon>
            <ListItemText primary="Indexes" slotProps={{ primary: { variant: 'body2' } }} />
          </ListItemButton>
        </ListItem>
      </List>
      <WorkloadTree />
    </Box>
  );
}
