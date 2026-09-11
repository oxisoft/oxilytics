<script>
  import { link, location } from 'svelte-spa-router';
  const items = [
    { path: '/settings', label: 'General' },
    { path: '/settings/stores', label: 'Stores' },
    { path: '/settings/users', label: 'Users' },
    { path: '/settings/ignored', label: 'Ignored apps' },
  ];
</script>

<nav class="mb-4 flex gap-1 border-b border-zinc-200 dark:border-zinc-800" aria-label="Settings">
  {#each items as it}
    <a href={it.path} use:link class="-mb-px border-b-2 px-3 py-2 text-sm {$location === it.path ? 'border-brand-600 font-medium' : 'border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'}">{it.label}</a>
  {/each}
</nav>
