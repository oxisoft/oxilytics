<script>
  import { link, location, push } from 'svelte-spa-router';
  import { api } from '../api.js';
  import { session } from '../session.svelte.js';
  import { theme } from '../theme.js';
  import Icon from './Icon.svelte';

  let menuOpen = $state(false);
  let userOpen = $state(false);

  const nav = $derived([
    { path: '/', label: 'Dashboard', icon: 'dashboard' },
    { path: '/products', label: 'Products', icon: 'products' },
    { path: '/reviews', label: 'Reviews', icon: 'reviews' },
    { path: '/sync', label: 'Sync', icon: 'sync' },
    ...(session.isAdmin ? [{ path: '/settings', label: 'Settings', icon: 'settings' }] : []),
  ]);

  function active(p) {
    const l = $location;
    return p === '/' ? l === '/' : l.startsWith(p);
  }

  async function logout() {
    await api.post('/auth/logout');
    session.clear();
    push('/login');
  }
</script>

<header class="sticky top-0 z-30 border-b border-zinc-200 bg-white/90 backdrop-blur dark:border-zinc-800 dark:bg-zinc-950/90">
  <div class="mx-auto flex h-14 max-w-7xl items-center gap-4 px-4">
    <a href="/" use:link class="flex items-center gap-2 font-semibold">
      <span class="grid h-7 w-7 place-items-center rounded-md bg-brand-600 text-white"><Icon name="dashboard" class="h-4 w-4" /></span>
      Oxilytics
    </a>

    <nav class="hidden items-center gap-1 md:flex" aria-label="Main">
      {#each nav as n}
        <a href={n.path} use:link class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm {active(n.path) ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-600 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800'}">
          <Icon name={n.icon} />{n.label}
        </a>
      {/each}
    </nav>

    <div class="ml-auto flex items-center gap-1">
      <button class="rounded-md p-2 text-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-800" onclick={theme.cycle} title="Theme: {theme.mode}" aria-label="Toggle theme">
        <Icon name={document.documentElement.classList.contains('dark') ? 'moon' : 'sun'} />
      </button>
      <div class="relative">
        <button class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-zinc-100 dark:hover:bg-zinc-800" onclick={() => (userOpen = !userOpen)} aria-haspopup="menu" aria-expanded={userOpen}>
          <span class="grid h-6 w-6 place-items-center rounded-full bg-brand-600 text-xs font-semibold text-white">{(session.user?.name || '?')[0].toUpperCase()}</span>
          <span class="hidden sm:inline">{session.user?.name}</span>
        </button>
        {#if userOpen}
          <div class="absolute right-0 mt-1 w-44 rounded-md border border-zinc-200 bg-white py-1 text-sm shadow-lg dark:border-zinc-700 dark:bg-zinc-900" role="menu" tabindex="-1" onmouseleave={() => (userOpen = false)}>
            <div class="px-3 py-1.5 text-xs text-zinc-500">{session.user?.email}<br /><span class="badge mt-1 bg-zinc-100 dark:bg-zinc-800">{session.user?.role}</span></div>
            <a href="/profile" use:link class="flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800" role="menuitem" onclick={() => (userOpen = false)}><Icon name="user" />My profile</a>
            <button class="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-zinc-100 dark:hover:bg-zinc-800" role="menuitem" onclick={logout}><Icon name="logout" />Sign out</button>
          </div>
        {/if}
      </div>
      <button class="rounded-md p-2 md:hidden" onclick={() => (menuOpen = !menuOpen)} aria-label="Menu"><Icon name="menu" /></button>
    </div>
  </div>
  {#if menuOpen}
    <nav class="border-t border-zinc-200 px-4 py-2 md:hidden dark:border-zinc-800">
      {#each nav as n}
        <a href={n.path} use:link class="flex items-center gap-2 rounded-md px-3 py-2 text-sm {active(n.path) ? 'bg-zinc-100 dark:bg-zinc-800' : ''}" onclick={() => (menuOpen = false)}><Icon name={n.icon} />{n.label}</a>
      {/each}
    </nav>
  {/if}
</header>
