import '@testing-library/jest-dom';

// jsdom does not implement matchMedia; MUI's useMediaQuery needs it. Default
// to "not matching" (desktop) — tests exercising a specific breakpoint
// override window.matchMedia themselves.
if (!window.matchMedia) {
  window.matchMedia = (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }) as MediaQueryList;
}
