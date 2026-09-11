<script>
  import Icon from './Icon.svelte';
  let { kind = 'error', message, onretry } = $props();
  const cls = { error: 'border-red-300 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200', warn: 'border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-200', info: 'border-sky-300 bg-sky-50 text-sky-800 dark:border-sky-900 dark:bg-sky-950/40 dark:text-sky-200' };
</script>

<div class="flex items-center gap-3 rounded-md border px-3 py-2 text-sm {cls[kind]}" role="alert">
  <Icon name={kind === 'info' ? 'info' : 'alert'} />
  <span class="flex-1">{message}</span>
  {#if onretry}<button class="underline" onclick={onretry}>Retry</button>{/if}
</div>
