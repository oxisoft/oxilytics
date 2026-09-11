<script>
  import { onMount } from 'svelte';
  import { push } from 'svelte-spa-router';
  import { api, qs } from '../api.js';
  import { filter } from '../filter.svelte.js';
  import { fmtDate, fmtDateTime, fmtRating, fmtNum, PLATFORM_LABEL } from '../format.js';
  import PlatformGlyph from './PlatformGlyph.svelte';
  import StoreBadge from './StoreBadge.svelte';
  import Stars from './Stars.svelte';
  import Skeleton from './Skeleton.svelte';
  import Modal from './Modal.svelte';
  import Icon from './Icon.svelte';

  // productId: fixed scope (product detail) or null (global page)
  let { productId = null, openId = null } = $props();
  let rows = $state([]);
  let total = $state(0);
  let page = $state(1);
  let stats = $state(null);
  let loading = $state(true);
  let ratings = $state([]);
  let q = $state('');
  let country = $state('');
  let replied = $state('');
  let detail = $state(null);
  let expanded = $state({});

  const params = $derived({ ...filter.params, product_id: productId ?? filter.params.product_id, rating: ratings.length ? ratings.join(',') : undefined, q: q || undefined, country: country || undefined, replied: replied || undefined });

  async function load() {
    loading = true;
    try {
      const [r, s] = await Promise.all([api.get('/reviews' + qs({ ...params, page, per_page: 50 })), api.get('/reviews/stats' + qs(params))]);
      rows = r.rows; total = r.total; stats = s;
    } finally { loading = false; }
  }
  onMount(load);
  $effect(() => { params; page; load(); });
  $effect(() => { if (openId) api.get('/reviews/' + openId).then((r) => (detail = r)).catch(() => {}); });

  function toggleRating(i) { ratings = ratings.includes(i) ? ratings.filter((x) => x !== i) : [...ratings, i]; page = 1; }
  const pages = $derived(Math.max(1, Math.ceil(total / 50)));
  const storeURL = (r) => r.store === 'appstore' ? `https://apps.apple.com/app/id${r.app_id}` : `https://play.google.com/store/apps/details?id=${r.app_name}`;
  const exportURL = $derived('/api/reviews/export.csv' + qs(params));
</script>

<div class="mb-3 flex flex-wrap items-center gap-2 text-sm">
  <div class="relative"><Icon name="search" class="absolute top-2 left-2 h-4 w-4 text-zinc-400" /><input class="input w-56 pl-8" placeholder="Search reviews…" bind:value={q} oninput={() => (page = 1)} /></div>
  <input class="input w-20" placeholder="Country" maxlength="2" bind:value={country} oninput={() => (page = 1)} />
  <select class="input w-auto" bind:value={replied} onchange={() => (page = 1)}><option value="">Replied: any</option><option value="1">Replied</option><option value="0">Not replied</option></select>
  <a class="btn-secondary ml-auto" href={exportURL} download><Icon name="download" />CSV</a>
</div>

{#if stats}
  <div class="card mb-3 flex flex-wrap items-center gap-4">
    <div><div class="text-2xl font-semibold">{fmtNum(stats.count)}</div><div class="text-xs text-zinc-500">reviews</div></div>
    <div><div class="text-2xl font-semibold text-amber-500">{stats.avg ? '★ ' + fmtRating(stats.avg) : '—'}</div><div class="text-xs text-zinc-500">average</div></div>
    <div class="flex flex-1 items-end gap-1" role="group" aria-label="Filter by stars">
      {#each [5, 4, 3, 2, 1] as i}
        {@const n = stats.histogram?.[i] || 0}
        <button class="flex flex-1 flex-col items-center gap-0.5 rounded px-1 py-1 text-xs {ratings.includes(i) ? 'bg-zinc-100 dark:bg-zinc-800' : 'hover:bg-zinc-50 dark:hover:bg-zinc-800/50'}" onclick={() => toggleRating(i)} aria-pressed={ratings.includes(i)}>
          <div class="flex h-10 w-full items-end"><div class="w-full rounded-t bg-amber-400" style="height:{stats.count ? Math.max(4, (n / stats.count) * 100) : 4}%"></div></div>
          <span>{i}★ <span class="text-zinc-400">{n}</span></span>
        </button>
      {/each}
    </div>
    <div class="text-xs text-zinc-500">{#each Object.entries(stats.by_platform || {}) as [p, s]}<div class="flex items-center gap-1"><PlatformGlyph platform={p} class="h-3 w-3" />{s.count} · {fmtRating(s.avg)}</div>{/each}</div>
  </div>
{/if}

{#if loading && !rows.length}<Skeleton rows={6} />
{:else if !rows.length}<p class="card py-10 text-center text-sm text-zinc-400">No reviews match.</p>
{:else}
  <ul class="card divide-y divide-zinc-100 p-0 dark:divide-zinc-800">
    {#each rows as r (r.id)}
      <li class="px-4 py-3">
        <div class="flex flex-wrap items-center gap-2 text-xs text-zinc-500">
          <PlatformGlyph platform={r.platform} /><Stars rating={r.rating} />
          <span class="font-medium text-zinc-700 dark:text-zinc-300">{r.app_name}</span>{#if r.app_version}<span>v{r.app_version}</span>{/if}
          {#if r.country}<span class="font-mono">{r.country}</span>{/if}
          <span>{fmtDate(r.created_at)}</span>
          {#if r.developer_reply}<span class="badge bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300">replied</span>{/if}
          <button class="ml-auto text-zinc-400 hover:underline" onclick={() => (detail = r)}>details</button>
        </div>
        {#if r.title}<div class="mt-1 text-sm font-medium">{r.title}</div>{/if}
        {#if r.body}<p class="mt-0.5 text-sm text-zinc-700 dark:text-zinc-300 {expanded[r.id] ? '' : 'line-clamp-3'}">{r.body}</p>
          {#if r.body.length > 240}<button class="text-xs text-zinc-400 hover:underline" onclick={() => (expanded[r.id] = !expanded[r.id])}>{expanded[r.id] ? 'less' : 'more'}</button>{/if}{/if}
        {#if r.author}<div class="mt-1 text-xs text-zinc-400">— {r.author}</div>{/if}
      </li>
    {/each}
  </ul>
  {#if pages > 1}
    <div class="mt-3 flex items-center justify-center gap-2 text-sm">
      <button class="btn-secondary" disabled={page <= 1} onclick={() => page--}>Prev</button><span>{page} / {pages}</span><button class="btn-secondary" disabled={page >= pages} onclick={() => page++}>Next</button>
    </div>
  {/if}
{/if}

<Modal open={!!detail} title="Review" onclose={() => { detail = null; if (openId) push('/reviews'); }}>
  {#if detail}
    <div class="mb-2 flex flex-wrap items-center gap-2 text-xs text-zinc-500"><PlatformGlyph platform={detail.platform} label /><StoreBadge store={detail.store} /><Stars rating={detail.rating} /><span>{fmtDateTime(detail.created_at)}</span>{#if detail.edited_at}<span>(edited {fmtDate(detail.edited_at)})</span>{/if}</div>
    {#if detail.title}<h3 class="font-medium">{detail.title}</h3>{/if}
    <p class="mt-1 text-sm whitespace-pre-wrap">{detail.body || '(no text)'}</p>
    <dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs text-zinc-500">
      <dt>App</dt><dd>{detail.app_name}</dd>
      {#if detail.author}<dt>Author</dt><dd>{detail.author}</dd>{/if}
      {#if detail.country}<dt>Country</dt><dd>{detail.country}</dd>{/if}
      {#if detail.language}<dt>Language</dt><dd>{detail.language}</dd>{/if}
      {#if detail.app_version}<dt>Version</dt><dd>{detail.app_version}</dd>{/if}
      {#if detail.device}<dt>Device</dt><dd>{detail.device}</dd>{/if}
    </dl>
    {#if detail.developer_reply}
      <div class="mt-3 rounded-md bg-zinc-50 p-3 text-sm dark:bg-zinc-800/60"><div class="mb-1 text-xs font-medium text-zinc-500">Developer reply · {fmtDate(detail.developer_replied_at)}</div>{detail.developer_reply}</div>
    {/if}
  {/if}
</Modal>
