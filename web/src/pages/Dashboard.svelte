<script>
  import { onMount } from 'svelte';
  import { link } from 'svelte-spa-router';
  import { api, qs } from '../lib/api.js';
  import { filter } from '../lib/filter.svelte.js';
  import { fmtNum, fmtCompact, fmtRating, fmtDate, fmtPct, PLATFORM_LABEL, productIcon } from '../lib/format.js';
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

  // Every product that had a download in the range, biggest first. Products
  // with zero downloads are dropped rather than listed as a tail of noughts;
  // the full list is still on the Products screen.
  //
  // Top performers is per PRODUCT, not per app. A product bundles its iOS and
  // Android apps, so listing apps showed the same product twice with its
  // downloads split across two rows.
  //
  // /products has no platform parameter, but each row carries by_platform, so
  // the bar's platform selection is honoured by reading that slice instead of
  // the portfolio total.
  //
  // toSorted keeps the copy: sorting (products || []) directly would reorder
  // the state array in place.
  function productDownloads(p) {
    if (filter.platform) return p.by_platform?.[filter.platform]?.downloads || 0;
    return p.totals?.downloads || 0;
  }
  const topProducts = $derived(
    (products || [])
      .map((p) => ({ row: p, downloads: productDownloads(p) }))
      .filter((x) => x.downloads > 0)
      .toSorted((x, y) => y.downloads - x.downloads)
  );
  const topMax = $derived(topProducts.length ? topProducts[0].downloads : 0);

  async function load() {
    loading = true; error = '';
    const p = filter.params;
    try {
      [summary, downloads, crashes, countries, products, reviews, sync] = await Promise.all([
        api.get('/metrics/summary' + qs(p)),
        api.get('/metrics/series' + qs({ ...p, metric: 'downloads', group, bucket })),
        api.get('/metrics/series' + qs({ ...p, metric: 'crashes', group: 'platform', bucket })),
        api.get('/metrics/countries' + qs({ ...p, limit: 10 })),
        // /products takes the date range only: it has no platform or product
        // parameter, so Top performers always covers the whole portfolio.
        // Filtering it by platform needs a server-side change.
        api.get('/products' + qs({ from: p.from, to: p.to })),
        api.get('/reviews' + qs({ ...p, per_page: 5 })),
        api.get('/sync/status'),
      ]);
    } catch (e) { error = e.message; }
    finally { loading = false; }
  }
  onMount(load);
  $effect(() => { filter.from; filter.to; filter.product; filter.platform; bucket; load(); });

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
  <!--
    The "N store apps are not linked to a product yet" banner lived here, on
    Products and on Reviews. Products → Unassigned already carries the count in
    its tab badge and explains the consequence, so repeating it on every screen
    was noise on a state the user has already seen and chosen to leave as is.
  -->

  {#if !hasData}
    <EmptyState icon="dashboard" title="No data yet" message="Run a sync to pull downloads, crashes and reviews from your stores, then link the discovered apps into products.">
      <a href="/sync" use:link class="btn-primary">Go to Sync</a>
    </EmptyState>
  {:else}
    <div class="mb-4 grid gap-4 lg:grid-cols-3">
      <!--
        Left column stacks the KPI cards on top of the downloads chart so the
        right column can run their full combined height: "all apps sorted by
        downloads" needs vertical room, and the two KPI slots freed by dropping
        the single-store cards are exactly that room.
      -->
      <div class="flex flex-col gap-4 lg:col-span-2">
        <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
          <KpiCard metric="downloads" label="Downloads" value={summary.totals.downloads} prev={summary.prev.downloads} split={Object.fromEntries(Object.entries(summary.by_platform).map(([k, v]) => [k, v.downloads]))} />
          <KpiCard metric="crashes" label="Crashes" value={summary.totals.crashes} prev={summary.prev.crashes} invert split={Object.fromEntries(Object.entries(summary.by_platform).map(([k, v]) => [k, v.crashes]))} />
          <KpiCard label="Avg review rating" value={summary.reviews?.avg ?? null} format={fmtRating} split={Object.fromEntries(Object.entries(summary.reviews?.by_platform || {}).map(([k, v]) => [k, v.avg]))} sub="reviews in range" />
          <KpiCard label="New reviews" value={summary.reviews?.count ?? 0} split={Object.fromEntries(Object.entries(summary.reviews?.by_platform || {}).map(([k, v]) => [k, v.count]))} />
        </div>
        <div class="card">
          <div class="mb-2 flex items-center justify-between">
            <h2 class="font-medium">Downloads</h2>
            <div class="flex gap-1 text-xs" role="group">
              {#each ['day', 'week', 'month'] as b}<button class="rounded px-2 py-0.5 {bucket === b ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-500'}" onclick={() => (bucket = b)}>{b}</button>{/each}
            </div>
          </div>
          {#if downloads?.series?.length}
            <ChartView type="line" labels={downloads.buckets} series={downloads.series.map((s) => ({ key: s.key, label: s.label, values: s.values }))} yFormat={fmtCompact} xUnit={bucket} />
          {:else}<p class="py-16 text-center text-sm text-zinc-400">No downloads in this range</p>{/if}
        </div>
      </div>

      <div class="card flex flex-col">
        <h2 class="mb-2 font-medium">Top performers</h2>
        <!--
          Per product, aggregated across its apps: a product can bundle an iOS
          and an Android app whose numbers come from different stores, and the
          question here is which product pulls downloads overall.
        -->
        {#if topProducts.length}
          <ul class="flex-1 space-y-0.5 overflow-y-auto pr-1 text-sm">
            {#each topProducts as t (t.row.product.id)}
              <li>
                <a href="/products/{t.row.product.id}" use:link class="-mx-1 flex items-center gap-2 rounded px-1 py-1 hover:bg-zinc-100 dark:hover:bg-zinc-800">
                  {#if productIcon(t.row)}
                    <img src={productIcon(t.row)} alt="" class="h-6 w-6 shrink-0 rounded" />
                  {:else}
                    <span class="grid h-6 w-6 shrink-0 place-items-center rounded bg-zinc-100 text-zinc-400 dark:bg-zinc-800"><Icon name="products" class="h-3 w-3" /></span>
                  {/if}
                  <span class="truncate" title={t.row.product.name}>{t.row.product.name}</span>
                  <!-- Which stores the number covers, so a product with one
                       platform is not silently compared against one with two.
                       With a platform selected the number is that platform
                       only, so show just that glyph. -->
                  <span class="flex shrink-0 items-center gap-0.5">
                    {#each t.row.apps.filter((a) => !filter.platform || a.platform === filter.platform) as a (a.id)}
                      <PlatformGlyph platform={a.platform} class="h-3 w-3 text-zinc-400" />
                    {/each}
                  </span>
                  <div class="ml-auto flex shrink-0 items-center gap-2">
                    <div class="hidden h-2 w-16 overflow-hidden rounded bg-zinc-100 sm:block dark:bg-zinc-800">
                      <div class="h-full bg-brand-500" style="width:{topMax ? (t.downloads / topMax) * 100 : 0}%"></div>
                    </div>
                    <span class="w-12 text-right tabular-nums">{fmtCompact(t.downloads)}</span>
                  </div>
                </a>
              </li>
            {/each}
          </ul>
        {:else}<p class="py-12 text-center text-sm text-zinc-400">No downloads in this range</p>{/if}
      </div>
    </div>

    <div class="mb-4 grid gap-4 lg:grid-cols-3">
      <div class="card">
        <h2 class="mb-2 font-medium">Crashes</h2>
        {#if crashes?.series?.length}
          <ChartView type="bar" stacked labels={crashes.buckets} series={crashes.series.map((s) => ({ key: s.key, label: PLATFORM_LABEL[s.key] || s.label, values: s.values }))} height={200} xUnit={bucket} />
        {:else}<p class="py-12 text-center text-sm text-zinc-400">No crashes reported</p>{/if}
      </div>
      <div class="card">
        <h2 class="mb-2 font-medium">Platform share</h2>
        {#if platformShare.length}
          <ChartView type="doughnut" labels={platformShare.map((s) => s.label)} series={[{ label: 'Downloads', values: platformShare.map((s) => s.values[0]), keys: platformShare.map((s) => s.key) }]} height={200} />
        {:else}<p class="py-12 text-center text-sm text-zinc-400">—</p>{/if}
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
                <td><a href="/products/{p.product.slug}" use:link class="flex items-center gap-2 font-medium hover:underline">{#if productIcon(p)}<img src={productIcon(p)} alt="" class="h-6 w-6 rounded" />{/if}{p.product.name}</a></td>
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
              <!-- Which app a review belongs to is the first thing you need; a
                   star rating with no app name is unactionable. -->
              <div class="mt-0.5 truncate text-xs font-medium text-zinc-700 dark:text-zinc-300">{r.app_name || '—'}</div>
              <div class="line-clamp-2 text-sm">{r.title || r.body || '(no text)'}</div>
            </a></li>
          {:else}<li class="py-6 text-center text-sm text-zinc-400">No reviews in range</li>{/each}
        </ul>
      </div>
    </div>
  {/if}
{/if}
