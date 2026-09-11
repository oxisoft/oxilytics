<script>
  let { rows = 3, class: cls = '' } = $props();
</script>

<div class="animate-pulse space-y-2 {cls}" aria-busy="true">
  {#each Array(rows) as _}
    <div class="h-4 rounded bg-zinc-200 dark:bg-zinc-800"></div>
  {/each}
</div>
