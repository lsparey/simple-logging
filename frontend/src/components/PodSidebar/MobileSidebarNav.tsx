import { useState } from 'react';
import Box from '@mui/material/Box';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import { useLocation, useNavigate } from 'react-router-dom';
import { useLogStore } from '../../store/logStore.js';
import SidebarSectionView from './SidebarSectionView.js';
import { SIDEBAR_SECTIONS, inferSectionFromPath, type SidebarSection } from './sidebarSections.js';

export const MOBILE_NAV_HEIGHT = 56;

export default function MobileSidebarNav() {
  const location = useLocation();
  const navigate = useNavigate();
  const { enterIndexMode, leaveIndexMode } = useLogStore();
  const [openSection, setOpenSection] = useState<SidebarSection | null>(null);
  const activeSection = inferSectionFromPath(location.pathname);

  function handleOpen(next: SidebarSection) {
    setOpenSection(next);
    if (next === 'indexes') {
      enterIndexMode();
      navigate('/indexes');
    } else {
      leaveIndexMode();
    }
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
        {SIDEBAR_SECTIONS.map(({ key, label, Icon }) => (
          <Tooltip key={key} title={label}>
            <IconButton
              aria-label={label}
              color={activeSection === key ? 'primary' : 'default'}
              onClick={() => handleOpen(key)}
            >
              <Icon />
            </IconButton>
          </Tooltip>
        ))}
      </Box>

      <Drawer
        anchor="bottom"
        open={openSection !== null}
        onClose={() => setOpenSection(null)}
        sx={{
          '& .MuiDrawer-paper': {
            height: 'calc(100% - 48px)',
          },
        }}
      >
        {openSection !== null && (
          <SidebarSectionView
            section={openSection}
            onBack={() => setOpenSection(null)}
            onLeafSelect={() => setOpenSection(null)}
          />
        )}
      </Drawer>
    </>
  );
}
