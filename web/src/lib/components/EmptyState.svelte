<script>
  import Icon from './Icon.svelte';
  let { icon = 'info', title = 'Nothing here', message = '', children } = $props();
</script>

<div class="card flex flex-col items-center py-12 text-center">
  <div class="mb-3 rounded-full bg-zinc-100 p-3 text-zinc-500 dark:bg-zinc-800"><Icon name={icon} class="h-6 w-6" /></div>
  <h3 class="text-sm font-semibold">{title}</h3>
  {#if message}<p class="mt-1 max-w-md text-sm text-zinc-500">{message}</p>{/if}
  {#if children}<div class="mt-4">{@render children()}</div>{/if}
</div>
