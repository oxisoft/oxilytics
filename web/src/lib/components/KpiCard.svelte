<script>
  import { fmtCompact, fmtPct, STORE_LABEL, metricCoverage } from '../format.js';
  import { session } from '../session.svelte.js';
  import PlatformGlyph from './PlatformGlyph.svelte';

  let {
    label,
    value,
    prev = null,
    split = null,
    format = fmtCompact,
    invert = false,
    sub = '',
    metric = '',
  } = $props();

  const delta = $derived(prev === null || prev === undefined || prev === 0 ? null : ((value - prev) / prev) * 100);
  const good = $derived(delta === null ? null : (invert ? delta < 0 : delta > 0));

  // Some metrics only exist in one store (Play has no updates column; Apple's
  // deletion report is weekly and sparse). Saying so on the card stops the
  // number being read as a portfolio-wide comparison.
  const coverage = $derived(metric ? metricCoverage(metric, session.setup) : { partial: false, stores: [] });
  const coverageText = $derived(
    coverage.partial ? coverage.stores.map((s) => STORE_LABEL[s] || s).join(' + ') + ' only' : ''
  );
</script>

<div class="card">
  <div class="flex items-center gap-1 text-xs font-medium text-zinc-500">
    <span>{label}</span>
    {#if coverageText}
      <span
        class="cursor-help rounded bg-zinc-100 px-1 text-[10px] font-normal text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400"
        title="{label} is only reported by {coverageText.replace(' only', '')}. The other configured store does not publish this metric, so this is not a cross-store total."
      >{coverageText}</span>
    {/if}
  </div>
  <div class="mt-1 flex items-baseline gap-2">
    <span class="text-2xl font-semibold tabular-nums">{format(value)}</span>
    {#if delta !== null}
      <span class="text-xs font-medium {good ? 'text-emerald-600' : 'text-red-600'}">{fmtPct(delta)}</span>
    {/if}
  </div>
  {#if sub}<div class="text-xs text-zinc-400">{sub}</div>{/if}
  {#if split && Object.keys(split).length}
    <div class="mt-2 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-zinc-500">
      {#each Object.entries(split) as [p, v]}
        <span class="inline-flex items-center gap-1"><PlatformGlyph platform={p} class="h-3 w-3" />{format(v)}</span>
      {/each}
    </div>
  {/if}
</div>
