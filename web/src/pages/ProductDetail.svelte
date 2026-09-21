<script>
  import { onMount } from 'svelte';
  import { link, push } from 'svelte-spa-router';
  import { api, qs } from '../lib/api.js';
  import { filter } from '../lib/filter.svelte.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { fmtNum, fmtCompact, fmtRating, PLATFORM_LABEL, STORE_LABEL, productIcon } from '../lib/format.js';
  import FilterBar from '../lib/components/FilterBar.svelte';
  import KpiCard from '../lib/components/KpiCard.svelte';
  import ChartView from '../lib/components/ChartView.svelte';
  import PlatformGlyph from '../lib/components/PlatformGlyph.svelte';
  import StoreBadge from '../lib/components/StoreBadge.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import Banner from '../lib/components/Banner.svelte';
  import Icon from '../lib/components/Icon.svelte';
  import ProductPicker from '../lib/components/ProductPicker.svelte';
  import IgnoreDialog from '../lib/components/IgnoreDialog.svelte';
  import ReviewsList from '../lib/components/ReviewsList.svelte';

  let { params = {} } = $props();
  let product = $state(null);
  let error = $state('');
  let tab = $state('overview');
  let summary = $state(null);
  let series = $state({});
  let countries = $state([]);
  let days = $state([]);
  let mode = $state('stacked'); // stacked | overlaid | total
  let unassigned = $state([]);
  let pickerApp = $state(null);
  let ignoreApp = $state(null);

  const storeURL = (a) => a.store === 'appstore' ? `https://apps.apple.com/app/id${a.store_app_id}` : `https://play.google.com/store/apps/details?id=${a.store_app_id}`;

  async function loadProduct() {
    error = '';
    try {
      product = await api.get('/products/' + params.slug);
      filter.product = String(product.id);
      if (params.platform) filter.platform = params.platform;
      if (session.isAdmin) unassigned = await api.get('/apps?unassigned=1');
    } catch (e) { error = e.status === 404 ? 'Product not found' : e.message; }
  }

  async function loadData() {
    if (!product) return;
    const p = { ...filter.params, product_id: product.id };
    try {
      const [s, dl, cr, up, un, co, dy] = await Promise.all([
        api.get('/metrics/summary' + qs(p)),
        api.get('/metrics/series' + qs({ ...p, metric: 'downloads', group: 'platform' })),
        api.get('/metrics/series' + qs({ ...p, metric: 'crashes', group: 'platform' })),
        api.get('/metrics/series' + qs({ ...p, metric: 'updates', group: 'platform' })),
        api.get('/metrics/series' + qs({ ...p, metric: 'uninstalls', group: 'platform' })),
        api.get('/metrics/countries' + qs({ ...p, by_platform: 1 })),
        api.get('/metrics/days' + qs(p)),
      ]);
      summary = s; series = { downloads: dl, crashes: cr, updates: up, uninstalls: un }; countries = co; days = dy;
    } catch (e) { error = e.message; }
  }

  onMount(loadProduct);
  $effect(() => { params.slug; loadProduct(); });
  $effect(() => { product; filter.from; filter.to; filter.platform; loadData(); });

  function chartSeries(s) {
    if (!s?.series) return [];
    if (mode === 'total') {
      const values = s.buckets.map((_, i) => s.series.reduce((acc, x) => acc + x.values[i], 0));
      return [{ key: 'total', label: 'Total', values }];
    }
    return s.series.map((x) => ({ key: x.key, label: PLATFORM_LABEL[x.key] || x.label, values: x.values }));
  }

  const countryTable = $derived.by(() => {
    const m = {};
    for (const r of countries) { m[r.country] ??= { country: r.country, total: 0 }; m[r.country][r.platform] = r.value; m[r.country].total += r.value; }
    return Object.values(m).sort((a, b) => b.total - a.total);
  });
  const platformsPresent = $derived([...new Set((product?.apps || []).map((a) => a.platform))]);

  async function unlink(a) {
    if (!confirm(`Unlink ${a.name} (${PLATFORM_LABEL[a.platform]}) from ${product.name}?`)) return;
    await api.del('/products/' + product.id + '/apps/' + a.id);
    toasts.success('Unlinked'); loadProduct();
  }
  const exportURL = $derived('/api/metrics/export.csv' + qs({ ...filter.params, product_id: product?.id }));
  const crashRate = (s) => s?.series ? s.buckets.map((_, i) => { const d = series.downloads?.series?.reduce((a, x) => a + x.values[i], 0) || 0; const c = s.series.reduce((a, x) => a + x.values[i], 0); return d ? +((c / d) * 1000).toFixed(2) : 0; }) : [];
</script>

{#if error}<Banner message={error} />
{:else if !product}<Skeleton rows={6} />
{:else}
  <a href="/products" use:link class="mb-2 inline-flex items-center gap-1 text-sm text-zinc-500 hover:underline"><Icon name="chevron" class="h-3.5 w-3.5 rotate-180" />Products</a>
  <div class="mb-4 flex flex-wrap items-center gap-4">
    {#if productIcon(product)}<img src={productIcon(product)} alt="" class="h-14 w-14 rounded-xl" />{/if}
    <div class="min-w-0 flex-1">
      <h1 class="text-xl font-semibold">{product.name}{#if product.archived}<span class="badge ml-2 bg-zinc-100 dark:bg-zinc-800">archived</span>{/if}</h1>
      {#if product.description}<p class="text-sm text-zinc-500">{product.description}</p>{/if}
      <div class="mt-1 flex flex-wrap gap-2">
        {#each ['ios', 'android', 'windows'] as plat}
          {@const a = product.apps.find((x) => x.platform === plat || (plat === 'ios' && x.platform === 'macos'))}
          {#if a}
            <button class="badge gap-1 border border-zinc-300 hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800 {filter.platform === a.platform ? 'bg-zinc-100 dark:bg-zinc-800' : ''}" onclick={() => filter.setPlatform(filter.platform === a.platform ? '' : a.platform)}>
              <PlatformGlyph platform={a.platform} class="h-3 w-3" />{PLATFORM_LABEL[a.platform]}{#if a.rating_avg}<span class="text-amber-500">★ {fmtRating(a.rating_avg)}</span>{/if}
            </button>
          {:else}
            <span class="badge gap-1 border border-dashed border-zinc-300 text-zinc-400 dark:border-zinc-700"><PlatformGlyph platform={plat} class="h-3 w-3" dim />{PLATFORM_LABEL[plat]} — {plat === 'windows' ? 'not yet' : 'no listing'}</span>
          {/if}
        {/each}
      </div>
    </div>
  </div>

  <FilterBar showProduct={false} />

  {#if summary}
    <div class="mb-4 grid grid-cols-2 gap-3 md:grid-cols-4">
      <KpiCard metric="downloads" label="Downloads" value={summary.totals.downloads} prev={summary.prev.downloads} split={Object.fromEntries(Object.entries(summary.by_platform).map(([k, v]) => [k, v.downloads]))} />
      <KpiCard metric="crashes" label="Crashes" value={summary.totals.crashes} prev={summary.prev.crashes} invert split={Object.fromEntries(Object.entries(summary.by_platform).map(([k, v]) => [k, v.crashes]))} />
      <KpiCard label="Avg review rating" value={summary.reviews?.avg ?? null} format={fmtRating} split={Object.fromEntries(Object.entries(summary.reviews?.by_platform || {}).map(([k, v]) => [k, v.avg]))} />
      <KpiCard label="Reviews" value={summary.reviews?.count ?? 0} split={Object.fromEntries(Object.entries(summary.reviews?.by_platform || {}).map(([k, v]) => [k, v.count]))} />
    </div>
  {/if}

  <nav class="mb-4 flex flex-wrap gap-1 border-b border-zinc-200 dark:border-zinc-800" aria-label="Tabs">
    {#each [['overview', 'Overview'], ['downloads', 'Downloads'], ['countries', 'Countries'], ['crashes', 'Crashes'], ['ratings', 'Ratings'], ['reviews', 'Reviews'], ...(session.isAdmin ? [['apps', 'Store apps']] : [])] as [k, label]}
      <button class="-mb-px border-b-2 px-3 py-2 text-sm {tab === k ? 'border-brand-600 font-medium' : 'border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'}" onclick={() => (tab = k)}>{label}</button>
    {/each}
    {#if ['overview', 'downloads', 'crashes'].includes(tab)}
      <div class="ml-auto flex items-center gap-1 self-center text-xs" role="group">
        {#each [['stacked', 'Stacked'], ['overlaid', 'Overlaid'], ['total', 'Total']] as [k, label]}<button class="rounded px-2 py-0.5 {mode === k ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-500'}" onclick={() => (mode = k)}>{label}</button>{/each}
      </div>
    {/if}
  </nav>

  {#if tab === 'overview'}
    <div class="grid gap-4 lg:grid-cols-3">
      <div class="card lg:col-span-2"><h2 class="mb-2 font-medium">Downloads</h2>{#if series.downloads}<ChartView type={mode === 'stacked' ? 'bar' : 'line'} stacked={mode === 'stacked'} labels={series.downloads.buckets} series={chartSeries(series.downloads)} yFormat={fmtCompact} />{/if}</div>
      <div class="card"><h2 class="mb-2 font-medium">Platform share</h2>{#if summary}<ChartView type="doughnut" labels={Object.keys(summary.by_platform).map((k) => PLATFORM_LABEL[k] || k)} series={[{ label: 'Downloads', values: Object.values(summary.by_platform).map((v) => v.downloads), keys: Object.keys(summary.by_platform) }]} />{/if}</div>
      <div class="card lg:col-span-2"><h2 class="mb-2 font-medium">Crash rate per 1k downloads</h2>{#if series.crashes}<ChartView type="line" labels={series.crashes.buckets} series={[{ key: 'rate', label: 'Crashes / 1k downloads', values: crashRate(series.crashes) }]} height={200} />{/if}</div>
      <div class="card"><h2 class="mb-2 font-medium">Rating per platform</h2>
        <ul class="space-y-2">{#each product.apps as a}<li class="flex items-center gap-2 text-sm"><PlatformGlyph platform={a.platform} />{PLATFORM_LABEL[a.platform]}<span class="ml-auto text-lg font-semibold text-amber-500">{a.rating_avg ? '★ ' + fmtRating(a.rating_avg) : '—'}</span>{#if a.rating_count}<span class="text-xs text-zinc-400">({fmtNum(a.rating_count)})</span>{/if}</li>{/each}</ul>
      </div>
    </div>
  {:else if tab === 'downloads'}
    <div class="space-y-4">
      {#each [['downloads', 'Downloads'], ['updates', 'Updates'], ['uninstalls', 'Uninstalls']] as [k, label]}
        <div class="card"><h2 class="mb-2 font-medium">{label}</h2>{#if series[k]}<ChartView type={mode === 'stacked' ? 'bar' : 'line'} stacked={mode === 'stacked'} labels={series[k].buckets} series={chartSeries(series[k])} yFormat={fmtCompact} height={200} />{/if}</div>
      {/each}
      <div class="card overflow-x-auto p-0">
        <div class="flex items-center justify-between px-3 py-2"><h2 class="font-medium">By day</h2><a class="btn-secondary" href={exportURL} download><Icon name="download" />CSV</a></div>
        <table class="table"><thead><tr><th>Day</th><th>Platform</th><th class="text-right">Downloads</th><th class="text-right">Redownloads</th><th class="text-right">Updates</th><th class="text-right">Uninstalls</th><th class="text-right">Active devices</th></tr></thead>
          <tbody>{#each days as d}<tr><td class="font-mono text-xs">{d.day}</td><td><PlatformGlyph platform={d.platform} label /></td><td class="text-right tabular-nums">{fmtNum(d.downloads)}</td><td class="text-right tabular-nums">{fmtNum(d.redownloads)}</td><td class="text-right tabular-nums">{fmtNum(d.updates)}</td><td class="text-right tabular-nums">{fmtNum(d.uninstalls)}</td><td class="text-right tabular-nums">{fmtNum(d.active_devices)}</td></tr>{:else}<tr><td colspan="7" class="py-6 text-center text-zinc-400">No data</td></tr>{/each}</tbody></table>
      </div>
    </div>
  {:else if tab === 'countries'}
    <div class="grid gap-4 lg:grid-cols-2">
      <div class="card"><h2 class="mb-2 font-medium">Top 15 countries by downloads</h2><ChartView type="bar" stacked labels={countryTable.slice(0, 15).map((c) => c.country)} series={platformsPresent.map((p) => ({ key: p, label: PLATFORM_LABEL[p], values: countryTable.slice(0, 15).map((c) => c[p] || 0) }))} height={360} /></div>
      <div class="card max-h-[420px] overflow-auto p-0"><table class="table"><thead class="sticky top-0 bg-white dark:bg-zinc-900"><tr><th>Country</th>{#each platformsPresent as p}<th class="text-right">{PLATFORM_LABEL[p]}</th>{/each}<th class="text-right">Total</th></tr></thead>
        <tbody>{#each countryTable as c}<tr><td class="font-mono text-xs">{c.country}</td>{#each platformsPresent as p}<td class="text-right tabular-nums">{fmtNum(c[p] || 0)}</td>{/each}<td class="text-right font-medium tabular-nums">{fmtNum(c.total)}</td></tr>{:else}<tr><td colspan="9" class="py-6 text-center text-zinc-400">No per-country data</td></tr>{/each}</tbody></table></div>
    </div>
  {:else if tab === 'crashes'}
    <div class="space-y-4">
      <div class="card"><h2 class="mb-2 font-medium">Crashes {#if platformsPresent.includes('android')}<span class="text-xs font-normal text-zinc-400">(Android crashes exclude ANRs)</span>{/if}</h2>{#if series.crashes}<ChartView type={mode === 'stacked' ? 'bar' : 'line'} stacked={mode === 'stacked'} labels={series.crashes.buckets} series={chartSeries(series.crashes)} />{/if}</div>
      <div class="card"><h2 class="mb-2 font-medium">Crash rate per 1k downloads</h2>{#if series.crashes}<ChartView type="line" labels={series.crashes.buckets} series={[{ key: 'rate', label: 'Crashes / 1k downloads', values: crashRate(series.crashes) }]} height={200} />{/if}</div>
    </div>
  {:else if tab === 'ratings'}
    <div class="grid gap-4 md:grid-cols-2">
      {#each product.apps as a}
        <div class="card"><div class="mb-2 flex items-center gap-2 font-medium"><PlatformGlyph platform={a.platform} />{PLATFORM_LABEL[a.platform]}<StoreBadge store={a.store} /></div>
          <div class="text-3xl font-semibold text-amber-500">{a.rating_avg ? '★ ' + fmtRating(a.rating_avg) : '—'}</div>
          <div class="text-xs text-zinc-500">{a.rating_count ? fmtNum(a.rating_count) + ' ratings' : 'ratings count not provided by store'}{#if a.rating_updated_at} · updated {new Date(a.rating_updated_at).toLocaleDateString()}{/if}</div>
          {#if summary?.reviews?.by_platform?.[a.platform]}<div class="mt-3 text-sm">Reviews in range: <b>{summary.reviews.by_platform[a.platform].count}</b>, average <b>{fmtRating(summary.reviews.by_platform[a.platform].avg)}</b></div>{/if}
        </div>
      {/each}
      {#if summary?.reviews}
        <div class="card md:col-span-2"><h2 class="mb-2 font-medium">Review stars in range</h2><ChartView type="bar" labels={['1★', '2★', '3★', '4★', '5★']} series={[{ label: 'Reviews', values: [1, 2, 3, 4, 5].map((i) => summary.reviews.histogram[i] || 0), color: '#f59e0b' }]} height={200} /></div>
      {/if}
    </div>
  {:else if tab === 'reviews'}
    <ReviewsList productId={product.id} />
  {:else if tab === 'apps'}
    <div class="card overflow-x-auto p-0">
      <table class="table"><thead><tr><th>Platform</th><th>Store</th><th>Name</th><th>Identifier</th><th>Last synced</th><th></th></tr></thead>
        <tbody>{#each product.apps as a}<tr><td><PlatformGlyph platform={a.platform} label /></td><td><StoreBadge store={a.store} /></td><td>{a.name}</td><td class="font-mono text-xs">{a.store_app_id}</td><td class="text-xs text-zinc-500">{a.last_synced_at ? new Date(a.last_synced_at).toLocaleString() : 'never'}</td>
          <td class="text-right whitespace-nowrap"><a class="btn-secondary" href={storeURL(a)} target="_blank" rel="noreferrer"><Icon name="external" />Store</a> <button class="btn-secondary" onclick={() => unlink(a)}><Icon name="unlink" />Unlink</button> <button class="btn-secondary" onclick={() => (ignoreApp = a)}><Icon name="ban" />Ignore</button></td></tr>{/each}</tbody></table>
      {#if unassigned.length}
        <div class="border-t border-zinc-100 p-3 dark:border-zinc-800">
          <h3 class="mb-1 text-sm font-medium">Link another platform</h3>
          <ul class="space-y-1">{#each unassigned.filter((u) => !product.apps.some((a) => a.platform === u.platform)) as u}<li class="flex items-center gap-2 text-sm"><PlatformGlyph platform={u.platform} />{u.name}<span class="text-xs text-zinc-500">{u.store_app_id}</span><button class="btn-secondary ml-auto" onclick={async () => { try { await api.post('/products/' + product.id + '/apps', { app_id: u.id }); toasts.success('Linked'); loadProduct(); } catch (e) { toasts.error(e.message); } }}><Icon name="link" />Link</button></li>{:else}<li class="text-xs text-zinc-400">No unassigned apps for missing platforms.</li>{/each}</ul>
        </div>
      {/if}
    </div>
  {/if}
{/if}

<IgnoreDialog app={ignoreApp} onclose={() => (ignoreApp = null)} ondone={() => { ignoreApp = null; loadProduct(); }} />
<ProductPicker app={pickerApp} onclose={() => (pickerApp = null)} onlinked={() => { pickerApp = null; loadProduct(); }} />
