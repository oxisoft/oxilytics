// Global data filter: product, platforms, date range. Mirrored into the hash query.
import { querystring, location } from 'svelte-spa-router';
import { get } from 'svelte/store';

const PRESETS = { '7d': 7, '30d': 30, '90d': 90, '12m': 365 };

function iso(d) { return d.toISOString().slice(0, 10); }

export function rangeFor(preset, today = new Date()) {
  const to = new Date(today);
  to.setUTCDate(to.getUTCDate() - 1); // stores lag; "to" is yesterday
  const from = new Date(to);
  if (preset === 'ytd') {
    from.setUTCMonth(0, 1);
  } else {
    from.setUTCDate(from.getUTCDate() - (PRESETS[preset] || 30) + 1);
  }
  return { from: iso(from), to: iso(to) };
}

class Filter {
  product = $state('');     // product id or ''
  platforms = $state([]);   // [] = all
  preset = $state('30d');
  from = $state(rangeFor('30d').from);
  to = $state(rangeFor('30d').to);

  get params() {
    return {
      product_id: this.product || undefined,
      platform: this.platforms.length ? this.platforms.join(',') : undefined,
      from: this.from,
      to: this.to,
    };
  }

  setPreset(p) {
    this.preset = p;
    if (p !== 'custom') {
      const r = rangeFor(p);
      this.from = r.from;
      this.to = r.to;
    }
    this.sync();
  }

  setCustom(from, to) {
    this.preset = 'custom';
    this.from = from;
    this.to = to;
    this.sync();
  }

  togglePlatform(p) {
    this.platforms = this.platforms.includes(p) ? this.platforms.filter((x) => x !== p) : [...this.platforms, p];
    this.sync();
  }

  setProduct(id) { this.product = id || ''; this.sync(); }

  // write current state into the hash query string
  sync() {
    const q = new URLSearchParams();
    if (this.product) q.set('product', this.product);
    if (this.platforms.length) q.set('platform', this.platforms.join(','));
    q.set('range', this.preset);
    if (this.preset === 'custom') { q.set('from', this.from); q.set('to', this.to); }
    const loc = get(location);
    history.replaceState(null, '', '#' + loc + '?' + q.toString());
  }

  // read from the hash query on load
  load() {
    const q = new URLSearchParams(get(querystring) || '');
    if (q.get('product')) this.product = q.get('product');
    if (q.get('platform')) this.platforms = q.get('platform').split(',').filter(Boolean);
    const r = q.get('range');
    if (r === 'custom' && q.get('from') && q.get('to')) {
      this.preset = 'custom'; this.from = q.get('from'); this.to = q.get('to');
    } else if (r && (PRESETS[r] || r === 'ytd')) {
      this.preset = r; const rr = rangeFor(r); this.from = rr.from; this.to = rr.to;
    }
  }
}

export const filter = new Filter();
