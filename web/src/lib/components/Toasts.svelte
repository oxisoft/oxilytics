<script>
  import { toasts } from '../toast.svelte.js';
  import Icon from './Icon.svelte';
  const cls = { info: 'bg-zinc-800 text-white', success: 'bg-emerald-600 text-white', error: 'bg-red-600 text-white' };
</script>

<div class="pointer-events-none fixed right-4 bottom-4 z-50 flex flex-col gap-2" aria-live="polite">
  {#each toasts.items as t (t.id)}
    <div class="pointer-events-auto flex items-center gap-3 rounded-md px-4 py-2 text-sm shadow-lg {cls[t.kind]}">
      <span>{t.message}</span>
      <button onclick={() => toasts.dismiss(t.id)} aria-label="Dismiss"><Icon name="x" class="h-3.5 w-3.5" /></button>
    </div>
  {/each}
</div>
