import { useState } from 'react';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import { useLocation, useNavigate } from 'react-router-dom';
import { useLogStore } from '../../store/logStore.js';
import SidebarSectionView from './SidebarSectionView.js';
import { SIDEBAR_SECTIONS, inferSectionFromPath, type SidebarSection } from './sidebarSections.js';

export default function PodSidebar() {
  const location = useLocation();
  const navigate = useNavigate();
  const { enterIndexMode, leaveIndexMode } = useLogStore();
  const [section, setSection] = useState<SidebarSection | null>(() => inferSectionFromPath(location.pathname));

  function openSection(next: SidebarSection) {
    setSection(next);
    if (next === 'indexes') {
      enterIndexMode();
      navigate('/indexes');
    } else {
      leaveIndexMode();
    }
  }

  function backToMenu() {
    setSection(null);
    navigate('/');
  }

  if (section === null) {
    return (
      <List disablePadding>
        {SIDEBAR_SECTIONS.map(({ key, label, Icon }) => (
          <ListItem key={key} disablePadding>
            <ListItemButton onClick={() => openSection(key)} dense>
              <ListItemIcon sx={{ minWidth: 32 }}>
                <Icon fontSize="small" />
              </ListItemIcon>
              <ListItemText primary={label} slotProps={{ primary: { variant: 'body2' } }} />
            </ListItemButton>
          </ListItem>
        ))}
      </List>
    );
  }

  return <SidebarSectionView section={section} onBack={backToMenu} />;
}
