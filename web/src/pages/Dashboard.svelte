<script>
  import { onMount } from 'svelte';
  import { link } from 'svelte-spa-router';
  import { api, qs } from '../lib/api.js';
  import { filter } from '../lib/filter.svelte.js';
  import { fmtNum, fmtCompact, fmtRating, fmtDate, fmtPct, PLATFORM_LABEL } from '../lib/format.js';
  import FilterBar from '../lib/components/FilterBar.svelte';
  import KpiCard from '../lib/components/KpiCard.svelte';
  import ChartView from '../lib/components/ChartView.svelte';
  import PlatformGlyph from '../lib/components/PlatformGlyph.svelte';
  import Stars from '../lib/components/Stars.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import Banner from '../lib/components/Banner.svelte';
  import ProgressBar from '../lib/components/ProgressBar.svelte';
  import Icon from '../lib/components/Icon.svelte';

  let summary = $state(null);
  let downloads = $state(null);
  let crashes = $state(null);
  let countries = $state(null);
  let products = $state(null);
  let reviews = $state(null);
  let sync = $state(null);
  let bucket = $state('day');
  let error = $state('');
  let loading = $state(true);

  const group = $derived(filter.product ? 'platform' : 'product');

  async function load() {
    loading = true; error = '';
    const p = filter.params;
    try {
      [summary, downloads, crashes, countries, products, reviews, sync] = await Promise.all([
        api.get('/metrics/summary' + qs(p)),
        api.get('/metrics/series' + qs({ ...p, metric: 'downloads', group, bucket })),
        api.get('/metrics/series' + qs({ ...p, metric: 'crashes', group: 'platform', bucket })),
        api.get('/metrics/countries' + qs({ ...p, limit: 10 })),
        api.get('/products' + qs({ from: p.from, to: p.to })),
        api.get('/reviews' + qs({ ...p, per_page: 5 })),
        api.get('/sync/status'),
      ]);
    } catch (e) { error = e.message; }
    finally { loading = false; }
  }
  onMount(load);
  $effect(() => { filter.from; filter.to; filter.product; filter.platforms.length; bucket; load(); });

  const hasData = $derived(summary && (summary.totals.downloads > 0 || summary.totals.updates > 0 || (summary.reviews?.count || 0) > 0 || (products || []).length > 0));
  const platformShare = $derived.by(() => {
    if (!summary) return [];
    return Object.entries(summary.by_platform || {}).map(([k, v]) => ({ key: k, label: PLATFORM_LABEL[k] || k, values: [v.downloads] }));
  });
  const runningRuns = $derived(Object.values(sync?.stores || {}).filter((s) => s.running));
  const failedRuns = $derived(Object.values(sync?.stores || {}).filter((s) => s.configured && s.last?.status === 'failed'));
  const deltaPct = (cur, prev) => (prev ? ((cur - prev) / prev) * 100 : null);
</script>

<FilterBar />

{#if error}<Banner message={error} onretry={load} />
{:else if loading && !summary}<Skeleton rows={8} />
{:else}
  {#each runningRuns as s}
    <div class="card mb-3 flex items-center gap-3">
      <Icon name="sync" class="h-4 w-4 animate-spin text-brand-600" />
      <div class="flex-1"><span class="text-sm">Syncing {s.store === 'appstore' ? 'App Store' : 'Google Play'} — {s.running.apps_done}/{s.running.apps_total} apps</span><ProgressBar value={s.running.apps_done} max={s.running.apps_total || 1} /></div>
      <a href="/sync/runs/{s.running.id}" use:link class="text-sm underline">Details</a>
    </div>
  {/each}
  {#each failedRuns as s}
    <div class="mb-3"><Banner kind="warn" message="Last {s.store === 'appstore' ? 'App Store' : 'Google Play'} sync failed: {s.last.error || 'unknown error'}" /></div>
  {/each}
  {#if sync?.unassigned_apps > 0}
    <div class="mb-3"><Banner kind="info" message="{sync.unassigned_apps} store app{sync.unassigned_apps > 1 ? 's are' : ' is'} not linked to a product yet — data from them is not counted in any product." /></div>
  {/if}

  {#if !hasData}
    <EmptyState icon="dashboard" title="No data yet" message="Run a sync to pull downloads, crashes and reviews from your stores, then link the discovered apps into products.">
      <a href="/sync" use:link class="btn-primary">Go to Sync</a>
    </EmptyState>
  {:else}
    <div class="mb-4 grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
      <KpiCard label="Downloads" value={summary.totals.downloads} prev={summary.prev.downloads} split={Object.fromEntries(Object.entries(summary.by_platform).map(([k, v]) => [k, v.downloads]))} />
      <KpiCard label="Updates" value={summary.totals.updates} prev={summary.prev.updates} split={Object.fromEntries(Object.entries(summary.by_platform).map(([k, v]) => [k, v.updates]))} />
      <KpiCard label="Uninstalls" value={summary.totals.uninstalls} prev={summary.prev.uninstalls} invert split={Object.fromEntries(Object.entries(summary.by_platform).map(([k, v]) => [k, v.uninstalls]))} />
      <KpiCard label="Crashes" value={summary.totals.crashes} prev={summary.prev.crashes} invert split={Object.fromEntries(Object.entries(summary.by_platform).map(([k, v]) => [k, v.crashes]))} />
      <KpiCard label="Avg review rating" value={summary.reviews?.avg ?? null} format={fmtRating} split={Object.fromEntries(Object.entries(summary.reviews?.by_platform || {}).map(([k, v]) => [k, v.avg]))} sub="reviews in range" />
      <KpiCard label="New reviews" value={summary.reviews?.count ?? 0} split={Object.fromEntries(Object.entries(summary.reviews?.by_platform || {}).map(([k, v]) => [k, v.count]))} />
    </div>

    <div class="mb-4 grid gap-4 lg:grid-cols-3">
      <div class="card lg:col-span-2">
        <div class="mb-2 flex items-center justify-between">
          <h2 class="font-medium">Downloads</h2>
          <div class="flex gap-1 text-xs" role="group">
            {#each ['day', 'week', 'month'] as b}<button class="rounded px-2 py-0.5 {bucket === b ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-500'}" onclick={() => (bucket = b)}>{b}</button>{/each}
          </div>
        </div>
        {#if downloads?.series?.length}
          <ChartView type="line" labels={downloads.buckets} series={downloads.series.map((s) => ({ key: s.key, label: s.label, values: s.values }))} yFormat={fmtCompact} />
        {:else}<p class="py-16 text-center text-sm text-zinc-400">No downloads in this range</p>{/if}
      </div>
      <div class="card">
        <h2 class="mb-2 font-medium">Platform share</h2>
        {#if platformShare.length}
          <ChartView type="doughnut" labels={platformShare.map((s) => s.label)} series={[{ label: 'Downloads', values: platformShare.map((s) => s.values[0]), keys: platformShare.map((s) => s.key) }]} />
        {:else}<p class="py-16 text-center text-sm text-zinc-400">—</p>{/if}
      </div>
    </div>

    <div class="mb-4 grid gap-4 lg:grid-cols-3">
      <div class="card lg:col-span-2">
        <h2 class="mb-2 font-medium">Crashes</h2>
        {#if crashes?.series?.length}
          <ChartView type="bar" stacked labels={crashes.buckets} series={crashes.series.map((s) => ({ key: s.key, label: PLATFORM_LABEL[s.key] || s.label, values: s.values }))} height={200} />
        {:else}<p class="py-12 text-center text-sm text-zinc-400">No crashes reported</p>{/if}
      </div>
      <div class="card">
        <h2 class="mb-2 font-medium">Top countries</h2>
        {#if countries?.rows?.length}
          <ul class="space-y-1.5 text-sm">
            {#each countries.rows as c}
              <li class="flex items-center gap-2">
                <span class="w-8 font-mono text-xs">{c.country}</span>
                <div class="h-2 flex-1 overflow-hidden rounded bg-zinc-100 dark:bg-zinc-800"><div class="h-full bg-brand-500" style="width:{countries.total ? (c.value / countries.total) * 100 : 0}%"></div></div>
                <span class="w-14 text-right tabular-nums">{fmtCompact(c.value)}</span>
                <span class="w-10 text-right text-xs text-zinc-400">{countries.total ? ((c.value / countries.total) * 100).toFixed(0) : 0}%</span>
              </li>
            {/each}
          </ul>
        {:else}<p class="py-12 text-center text-sm text-zinc-400">No per-country data</p>{/if}
      </div>
    </div>

    <div class="mb-4 grid gap-4 lg:grid-cols-3">
      <div class="card overflow-x-auto p-0 lg:col-span-2">
        <table class="table">
          <thead><tr><th>Product</th><th>Platforms</th><th class="text-right">Downloads</th><th class="text-right">Δ</th><th class="text-right">Crashes</th><th class="text-right">Rating</th><th>Last review</th></tr></thead>
          <tbody>
            {#each products || [] as p (p.product.id)}
              <tr>
                <td><a href="/products/{p.product.slug}" use:link class="flex items-center gap-2 font-medium hover:underline">{#if p.product.icon_url}<img src={p.product.icon_url} alt="" class="h-6 w-6 rounded" />{/if}{p.product.name}</a></td>
                <td class="whitespace-nowrap">{#each p.apps as a}<span class="mr-1"><PlatformGlyph platform={a.platform} /></span>{/each}</td>
                <td class="text-right tabular-nums">{fmtNum(p.totals.downloads)}<div class="text-xs text-zinc-400">{#each Object.entries(p.by_platform) as [k, v]}<span class="mr-1">{PLATFORM_LABEL[k]?.[0] || k[0]} {fmtCompact(v.downloads)}</span>{/each}</div></td>
                <td class="text-right text-xs {deltaPct(p.totals.downloads, p.prev.downloads) > 0 ? 'text-emerald-600' : deltaPct(p.totals.downloads, p.prev.downloads) < 0 ? 'text-red-600' : 'text-zinc-400'}">{fmtPct(deltaPct(p.totals.downloads, p.prev.downloads))}</td>
                <td class="text-right tabular-nums">{fmtNum(p.totals.crashes)}</td>
                <td class="text-right">{#each p.apps as a}{#if a.rating_avg}<div class="text-xs"><PlatformGlyph platform={a.platform} class="inline h-3 w-3" /> {fmtRating(a.rating_avg)}</div>{/if}{/each}</td>
                <td class="text-xs text-zinc-500">{fmtDate(p.last_review_at)}</td>
              </tr>
            {:else}
              <tr><td colspan="7" class="py-8 text-center text-zinc-400">No products yet — <a href="/products" use:link class="underline">link your store apps</a></td></tr>
            {/each}
          </tbody>
        </table>
      </div>
      <div class="card">
        <h2 class="mb-2 font-medium">Recent reviews</h2>
        <ul class="divide-y divide-zinc-100 dark:divide-zinc-800">
          {#each reviews?.rows || [] as r}
            <li class="py-2"><a href="/reviews/{r.id}" use:link class="block hover:underline">
              <div class="flex items-center gap-2 text-xs text-zinc-500"><PlatformGlyph platform={r.platform} class="h-3 w-3" /><Stars rating={r.rating} /><span>{fmtDate(r.created_at)}</span></div>
              <div class="line-clamp-2 text-sm">{r.title || r.body || '(no text)'}</div>
            </a></li>
          {:else}<li class="py-6 text-center text-sm text-zinc-400">No reviews in range</li>{/each}
        </ul>
      </div>
    </div>
  {/if}
{/if}
