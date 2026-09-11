<script>
  let { value = 0, max = 100, label = '' } = $props();
  const pct = $derived(max > 0 ? Math.min(100, Math.round((value / max) * 100)) : 0);
</script>

<div class="w-full" role="progressbar" aria-valuenow={value} aria-valuemax={max} aria-label={label}>
  <div class="h-2 w-full overflow-hidden rounded-full bg-zinc-200 dark:bg-zinc-800">
    <div class="h-full bg-brand-500 transition-all" style="width:{pct}%"></div>
  </div>
  {#if label}<div class="mt-1 text-xs text-zinc-500">{label}</div>{/if}
</div>
