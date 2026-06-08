import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import Drawer from '@mui/material/Drawer';
import Box from '@mui/material/Box';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import Brightness4Icon from '@mui/icons-material/Brightness4';
import Brightness7Icon from '@mui/icons-material/Brightness7';
import AnalyticsIcon from '@mui/icons-material/Analytics';
import { useLocation, useNavigate } from 'react-router-dom';
import PodSidebar from '../PodSidebar/PodSidebar.js';
import LogPanel from '../LogPanel/LogPanel.js';
import IndexPanel from '../LogPanel/IndexPanel.js';
import DataDashboard from '../DataDashboard/DataDashboard.js';
import { useLogStore } from '../../store/logStore.js';

const DRAWER_WIDTH = 260;

export default function AppShell() {
  const { darkMode, toggleDarkMode, selectionKey, selectedIndexKey } = useLogStore();
  const location = useLocation();
  const navigate = useNavigate();
  const dashboardOpen = /^\/dashboard\/?$/.test(location.pathname);

  return (
    <Box sx={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <AppBar
        position="fixed"
        elevation={0}
        sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}
      >
        <Toolbar variant="dense">
          <Box sx={{ display: 'flex', alignItems: 'center', flexGrow: 1, gap: 1 }}>
            <img src="/logo.svg" alt="logo" style={{ height: 28, width: 28 }} />
            <Typography variant="h6" component="div" sx={{ fontFamily: 'monospace' }}>
              simple-logging
            </Typography>
          </Box>
          <Tooltip title="Data dashboard">
            <IconButton
              aria-label="Open data dashboard"
              color="inherit"
              onClick={() => navigate('/dashboard')}
              size="small"
              sx={{ mr: 0.5 }}
            >
              <AnalyticsIcon />
            </IconButton>
          </Tooltip>
          <Tooltip title={darkMode ? 'Use light theme' : 'Use dark theme'}>
            <IconButton aria-label="Toggle brightness" color="inherit" onClick={toggleDarkMode} size="small">
              {darkMode ? <Brightness7Icon /> : <Brightness4Icon />}
            </IconButton>
          </Tooltip>
        </Toolbar>
      </AppBar>

      <Drawer
        variant="permanent"
        sx={{
          width: DRAWER_WIDTH,
          flexShrink: 0,
          '& .MuiDrawer-paper': {
            width: DRAWER_WIDTH,
            boxSizing: 'border-box',
            top: '48px',  // below AppBar (dense = 48px)
            height: 'calc(100% - 48px)',
          },
        }}
      >
        <PodSidebar />
      </Drawer>

      <Box
        component="main"
        sx={{
          flexGrow: 1,
          display: 'flex',
          flexDirection: 'column',
          height: '100vh',
          pt: '48px',  // dense AppBar height
          overflow: 'hidden',
        }}
      >
        {dashboardOpen
          ? <DataDashboard />
          : selectedIndexKey !== null
            ? <IndexPanel key={selectionKey} />
            : <LogPanel key={selectionKey} />}
      </Box>
    </Box>
  );
}
