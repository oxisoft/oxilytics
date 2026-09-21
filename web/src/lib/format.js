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

// Which store actually reports which metric.
//
// The stores do not publish the same things, so a headline total can silently
// mean "one platform only":
//   - Google Play's install reports have NO updates column at all.
//   - Apple's Installation and Deletion report is published WEEKLY and only for
//     apps with enough volume, so deletions are near-absent for a small
//     portfolio.
//   - Active devices is a Play concept; Apple has no equivalent.
// Presenting those side by side as portfolio totals invites a false comparison
// ("uninstalls are an Android problem") when the truth is "the other store
// never told us". Metrics are still shown — hiding data would be worse — but
// the UI labels which stores stand behind each number.
export const METRIC_STORES = {
  downloads: ['appstore', 'googleplay'],
  updates: ['appstore'],
  uninstalls: ['googleplay'],
  active_devices: ['googleplay'],
  crashes: ['appstore', 'googleplay'],
  anrs: ['googleplay'],
  reviews: ['appstore', 'googleplay'],
};

// Stores that are configured AND cover this metric, given /setup/status.
export function metricCoverage(metric, setup) {
  const stores = METRIC_STORES[metric] || [];
  const configured = Object.entries(setup?.stores || {})
    .filter(([, v]) => v.configured)
    .map(([k]) => k);
  if (!configured.length) return { partial: false, stores: [] };
  const covering = stores.filter((s) => configured.includes(s));
  // Partial only when a configured store is missing from the metric — with a
  // single store configured there is nothing to mislead anyone about.
  return { partial: configured.length > 1 && covering.length < configured.length, stores: covering };
}

// A product's icon, falling back to the first store app that has one.
//
// Most products have no icon_url of their own — it is an optional override —
// so the product list and dashboard showed a grey placeholder while the detail
// page, which already applied this fallback inline, showed the real icon. Same
// product, different icon depending on the screen. Keep the rule in one place.
export function productIcon(row) {
  if (!row) return null;
  const p = row.product || row;
  if (p.icon_url) return p.icon_url;
  return (row.apps || []).find((a) => a.icon_url)?.icon_url || null;
}
