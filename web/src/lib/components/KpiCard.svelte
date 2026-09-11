<script>
  import { fmtCompact, fmtPct, PLATFORM_LABEL } from '../format.js';
  import PlatformGlyph from './PlatformGlyph.svelte';
  let { label, value, prev = null, split = null, format = fmtCompact, invert = false, sub = '' } = $props();
  const delta = $derived(prev === null || prev === undefined || prev === 0 ? null : ((value - prev) / prev) * 100);
  const good = $derived(delta === null ? null : (invert ? delta < 0 : delta > 0));
</script>

<div class="card">
  <div class="text-xs font-medium text-zinc-500">{label}</div>
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
