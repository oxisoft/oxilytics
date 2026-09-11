const KEY = 'oxi.theme';

function current() {
  return localStorage.getItem(KEY) || 'system';
}

function isDark(mode) {
  if (mode === 'dark') return true;
  if (mode === 'light') return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

export const theme = {
  get mode() { return current(); },
  apply() {
    document.documentElement.classList.toggle('dark', isDark(current()));
  },
  set(mode) {
    localStorage.setItem(KEY, mode);
    theme.apply();
  },
  cycle() {
    const order = ['system', 'light', 'dark'];
    theme.set(order[(order.indexOf(current()) + 1) % order.length]);
  },
};

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => theme.apply());
