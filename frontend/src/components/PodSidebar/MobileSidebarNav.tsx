import { useState } from 'react';
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
import SidebarSectionView from './SidebarSectionView.js';
import { WORKLOAD_KIND_SECTIONS, inferSectionFromPath, type WorkloadKind } from './sidebarSections.js';

export const MOBILE_NAV_HEIGHT = 56;

// null = closed, 'kindPicker' = choosing a workload kind, otherwise the open section.
type MobileView = null | 'indexes' | 'kindPicker' | WorkloadKind;

export default function MobileSidebarNav() {
  const location = useLocation();
  const navigate = useNavigate();
  const { enterIndexMode, leaveIndexMode } = useLogStore();
  const [view, setView] = useState<MobileView>(null);

  const activeSection = inferSectionFromPath(location.pathname);
  const indexesActive = activeSection === 'indexes';
  const workloadsActive = activeSection !== null && activeSection !== 'indexes';

  function openIndexes() {
    setView('indexes');
    enterIndexMode();
    navigate('/indexes');
  }

  function openWorkloadsMenu() {
    setView('kindPicker');
    leaveIndexMode();
  }

  function openKind(kind: WorkloadKind) {
    setView(kind);
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
          <IconButton aria-label="Indexes" color={indexesActive ? 'primary' : 'default'} onClick={openIndexes}>
            <KeyIcon />
          </IconButton>
        </Tooltip>
        <Tooltip title="Workloads">
          <IconButton aria-label="Workloads" color={workloadsActive ? 'primary' : 'default'} onClick={openWorkloadsMenu}>
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
        {view === 'kindPicker' && (
          <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
            <List disablePadding>
              <ListItem disablePadding sx={{ borderBottom: 1, borderColor: 'divider' }}>
                <ListItemButton onClick={closeDrawer} dense aria-label="Back">
                  <ListItemIcon sx={{ minWidth: 32 }}>
                    <ArrowBackIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText primary="Workloads" slotProps={{ primary: { variant: 'body2' } }} />
                </ListItemButton>
              </ListItem>
            </List>
            <List disablePadding>
              {WORKLOAD_KIND_SECTIONS.map(({ key, label, Icon }) => (
                <ListItem key={key} disablePadding>
                  <ListItemButton onClick={() => openKind(key)} dense>
                    <ListItemIcon sx={{ minWidth: 32 }}>
                      <Icon fontSize="small" />
                    </ListItemIcon>
                    <ListItemText primary={label} slotProps={{ primary: { variant: 'body2' } }} />
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
        )}

        {view === 'indexes' && (
          <SidebarSectionView section="indexes" onBack={closeDrawer} onLeafSelect={closeDrawer} />
        )}

        {view !== null && view !== 'kindPicker' && view !== 'indexes' && (
          <SidebarSectionView section={view} onBack={openWorkloadsMenu} onLeafSelect={closeDrawer} />
        )}
      </Drawer>
    </>
  );
}
