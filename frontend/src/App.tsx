import { ThemeProvider } from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import { useLogStore } from './store/logStore.js';
import { lightTheme, darkTheme } from './theme.js';
import AppShell from './components/AppShell/AppShell.js';

export default function App() {
  const darkMode = useLogStore((s) => s.darkMode);
  return (
    <ThemeProvider theme={darkMode ? darkTheme : lightTheme}>
      <CssBaseline />
      <AppShell />
    </ThemeProvider>
  );
}
