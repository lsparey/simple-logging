import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App.js';
import ErrorBoundary from './components/ErrorBoundary.js';
import { useLogStore } from './store/logStore.js';

// Initialise selection from the URL before first render so that the sidebar can
// auto-expand the right namespace on load.
(function initStoreFromUrl() {
  const pathname = window.location.pathname;
  const indexMatch = pathname.match(/^\/index\/([^/]+)\/?$/);
  const indexesMatch = pathname.match(/^\/indexes\/?$/);
  const workloadMatch = pathname.match(/^\/ns\/([^/]+)\/([^/]+)\/([^/]+)\/?$/);
  const nsKindMatch = pathname.match(/^\/ns\/([^/]+)\/([^/]+)\/?$/);
  const nsOnlyMatch = pathname.match(/^\/ns\/([^/]+)\/?$/);
  if (indexMatch) {
    useLogStore.getState().setSelectedIndex(decodeURIComponent(indexMatch[1]));
  } else if (indexesMatch) {
    useLogStore.getState().enterIndexMode();
  } else if (workloadMatch) {
    useLogStore.getState().setSelectedWorkload(
      decodeURIComponent(workloadMatch[1]),
      decodeURIComponent(workloadMatch[2]),
      decodeURIComponent(workloadMatch[3]),
    );
  } else if (nsKindMatch) {
    useLogStore.setState({ selectedNamespace: decodeURIComponent(nsKindMatch[1]) });
  } else if (nsOnlyMatch) {
    useLogStore.setState({ selectedNamespace: decodeURIComponent(nsOnlyMatch[1]) });
  }
})();

// Global handler for uncaught errors and unhandled promise rejections.
// Covers async errors that React's error boundary cannot catch (event handlers,
// gRPC stream callbacks, setTimeout, etc.) and errors that occur before React
// mounts at all.
function showGlobalError(message: string) {
  const existing = document.getElementById('__global-error__');
  if (existing) {
    existing.remove();
  }
  const el = document.createElement('div');
  el.id = '__global-error__';
  Object.assign(el.style, {
    position: 'fixed', inset: '0', zIndex: '99999',
    display: 'flex', flexDirection: 'column',
    alignItems: 'center', justifyContent: 'center',
    background: '#fff', fontFamily: 'monospace', padding: '2rem', gap: '1rem',
  });
  el.innerHTML = `
    <h2 style="margin:0;color:#333">Something went wrong</h2>
    <pre style="background:#f5f5f5;padding:1rem;border-radius:4px;max-width:80vw;overflow:auto;font-size:0.8rem;color:#c62828;white-space:pre-wrap">${message}</pre>
    <button onclick="window.location.reload()" style="padding:0.5rem 1rem;cursor:pointer">Reload</button>
  `;
  document.body.appendChild(el);
}

window.onerror = (_event, _source, _lineno, _colno, error) => {
  console.error('[global onerror]', error);
  showGlobalError(error?.message ?? String(error));
  return false;
};

window.addEventListener('unhandledrejection', (event) => {
  const reason = event.reason;
  console.error('[unhandledrejection]', reason);
  showGlobalError(reason instanceof Error ? reason.message : String(reason));
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ErrorBoundary>
  </StrictMode>,
);
