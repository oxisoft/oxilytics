export function fmtNum(n) {
  if (n === null || n === undefined) return '—';
  return new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(n);
}

export function fmtCompact(n) {
  if (n === null || n === undefined) return '—';
  return new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 }).format(n);
}

export function fmtPct(n, digits = 1) {
  if (n === null || n === undefined || !isFinite(n)) return '—';
  const s = (n > 0 ? '+' : '') + n.toFixed(digits) + '%';
  return s;
}

export function fmtRating(n) {
  if (n === null || n === undefined) return '—';
  return Number(n).toFixed(2);
}

export function fmtDate(s) {
  if (!s) return '—';
  const d = new Date(s);
  if (isNaN(d)) return s;
  return d.toLocaleDateString('en', { year: 'numeric', month: 'short', day: 'numeric' });
}

export function fmtDateTime(s) {
  if (!s) return '—';
  const d = new Date(s);
  if (isNaN(d)) return s;
  return d.toLocaleString('en', { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

export function fmtDuration(from, to) {
  if (!from) return '—';
  const ms = (to ? new Date(to) : new Date()) - new Date(from);
  if (ms < 1000) return '<1s';
  const s = Math.floor(ms / 1000);
  if (s < 60) return s + 's';
  const m = Math.floor(s / 60);
  if (m < 60) return m + 'm ' + (s % 60) + 's';
  const h = Math.floor(m / 60);
  return h + 'h ' + (m % 60) + 'm';
}

export function ago(s) {
  if (!s) return 'never';
  const diff = (Date.now() - new Date(s)) / 1000;
  if (diff < 60) return 'just now';
  if (diff < 3600) return Math.floor(diff / 60) + ' min ago';
  if (diff < 86400) return Math.floor(diff / 3600) + ' h ago';
  return Math.floor(diff / 86400) + ' d ago';
}

export const PLATFORM_LABEL = { ios: 'iOS', macos: 'macOS', android: 'Android', windows: 'Windows' };
export const STORE_LABEL = { appstore: 'App Store', googleplay: 'Google Play', msstore: 'Microsoft Store' };
export const STORE_PLATFORMS = { appstore: ['ios', 'macos'], googleplay: ['android'] };
