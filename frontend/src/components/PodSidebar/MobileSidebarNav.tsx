import { useState, type ReactNode } from 'react';
import Box from '@mui/material/Box';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import KeyIcon from '@mui/icons-material/Key';
import CategoryIcon from '@mui/icons-material/Category';
import { useLocation, useNavigate } from 'react-router-dom';
import { useLogStore } from '../../store/logStore.js';
import WorkloadTree from './WorkloadTree.js';
import IndexSidebar from './IndexSidebar.js';
import { inferAreaFromPath } from './sidebarSections.js';

export const MOBILE_NAV_HEIGHT = 56;

type MobileView = null | 'indexes' | 'workloads';

function DrawerPane({ label, onBack, children }: { label: string; onBack: () => void; children: ReactNode }) {
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <List disablePadding>
        <ListItem disablePadding sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <ListItemButton onClick={onBack} dense aria-label="Back">
            <ListItemIcon sx={{ minWidth: 32 }}>
              <ArrowBackIcon fontSize="small" />
            </ListItemIcon>
            <ListItemText primary={label} slotProps={{ primary: { variant: 'body2' } }} />
          </ListItemButton>
        </ListItem>
      </List>
      <Box sx={{ overflow: 'auto', flex: 1 }}>{children}</Box>
    </Box>
  );
}

export default function MobileSidebarNav() {
  const location = useLocation();
  const navigate = useNavigate();
  const { enterIndexMode, leaveIndexMode } = useLogStore();
  const [view, setView] = useState<MobileView>(null);

  const activeArea = inferAreaFromPath(location.pathname);

  function openIndexes() {
    setView('indexes');
    enterIndexMode();
    navigate('/indexes');
  }

  function openWorkloads() {
    setView('workloads');
    leaveIndexMode();
  }

  function closeDrawer() {
    setView(null);
  }

  return (
    <>
      <Box
        sx={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          height: MOBILE_NAV_HEIGHT,
          display: 'flex',
          justifyContent: 'space-around',
          alignItems: 'center',
          bgcolor: 'background.paper',
          borderTop: 1,
          borderColor: 'divider',
          zIndex: (theme) => theme.zIndex.appBar,
        }}
      >
        <Tooltip title="Indexes">
          <IconButton aria-label="Indexes" color={activeArea === 'indexes' ? 'primary' : 'default'} onClick={openIndexes}>
            <KeyIcon />
          </IconButton>
        </Tooltip>
        <Tooltip title="Workloads">
          <IconButton aria-label="Workloads" color={activeArea === 'workloads' ? 'primary' : 'default'} onClick={openWorkloads}>
            <CategoryIcon />
          </IconButton>
        </Tooltip>
      </Box>

      <Drawer
        anchor="bottom"
        open={view !== null}
        onClose={closeDrawer}
        sx={{
          '& .MuiDrawer-paper': {
            height: 'calc(100% - 48px)',
          },
        }}
      >
        {view === 'indexes' && (
          <DrawerPane label="Indexes" onBack={closeDrawer}>
            <IndexSidebar onLeafSelect={closeDrawer} />
          </DrawerPane>
        )}
        {view === 'workloads' && (
          <DrawerPane label="Workloads" onBack={closeDrawer}>
            <WorkloadTree onLeafSelect={closeDrawer} />
          </DrawerPane>
        )}
      </Drawer>
    </>
  );
}
