<script>
  import Router, { push, location } from 'svelte-spa-router';
  import { onMount } from 'svelte';
  import { api } from './lib/api.js';
  import { session } from './lib/session.svelte.js';
  import TopBar from './lib/components/TopBar.svelte';
  import Toasts from './lib/components/Toasts.svelte';
  import Footer from './lib/components/Footer.svelte';

  import Login from './pages/Login.svelte';
  import LoginTOTP from './pages/LoginTOTP.svelte';
  import Setup from './pages/Setup.svelte';
  import StoreGuide from './pages/StoreGuide.svelte';
  import Dashboard from './pages/Dashboard.svelte';
  import Products from './pages/Products.svelte';
  import ProductDetail from './pages/ProductDetail.svelte';
  import Apps from './pages/Apps.svelte';
  import Reviews from './pages/Reviews.svelte';
  import Sync from './pages/Sync.svelte';
  import SyncRun from './pages/SyncRun.svelte';
  import Settings from './pages/Settings.svelte';
  import Users from './pages/Users.svelte';
  import SettingsStores from './pages/SettingsStores.svelte';
  import IgnoredApps from './pages/IgnoredApps.svelte';
  import Profile from './pages/Profile.svelte';
  import NotFound from './pages/NotFound.svelte';

  const routes = {
    '/login': Login,
    '/login/totp': LoginTOTP,
    '/setup': Setup,
    '/setup/:store': StoreGuide,
    '/': Dashboard,
    '/products': Products,
    '/products/:slug': ProductDetail,
    '/products/:slug/:platform': ProductDetail,
    '/apps': Apps,
    '/reviews': Reviews,
    '/reviews/:id': Reviews,
    '/sync': Sync,
    '/sync/runs/:id': SyncRun,
    '/settings': Settings,
    '/settings/users': Users,
    '/settings/stores': SettingsStores,
    '/settings/ignored': IgnoredApps,
    '/profile': Profile,
    '*': NotFound,
  };

  const publicPaths = ['/login', '/login/totp'];

  onMount(async () => {
    try {
      const [me, v, h] = await Promise.all([
        api.get('/me', { redirectOn401: false }),
        api.get('/version'),
        api.get('/health'),
      ]);
      session.set(me);
      session.version = v;
      session.setupRequired = !!h.setup_required;
    } catch (e) {
      session.clear();
      try { session.version = await api.get('/version'); } catch {}
    }
  });

  // route guards
  $effect(() => {
    if (!session.loaded) return;
    const path = $location;
    const isPublic = publicPaths.includes(path);
    if (!session.authenticated && !isPublic) {
      push('/login?next=' + encodeURIComponent(path));
      return;
    }
    if (session.authenticated && isPublic) {
      push(session.setupRequired ? '/setup' : '/');
      return;
    }
    if (session.authenticated && session.setupRequired && !path.startsWith('/setup') && !path.startsWith('/settings') && path !== '/profile') {
      push('/setup');
    }
  });

  const showChrome = $derived(session.authenticated && !publicPaths.includes($location));
</script>

<div class="flex min-h-full flex-col">
  {#if showChrome}<TopBar />{/if}
  <main class="mx-auto w-full max-w-7xl flex-1 px-4 py-6">
    {#if session.loaded}
      <Router {routes} />
    {/if}
  </main>
  {#if showChrome}<Footer />{/if}
</div>
<Toasts />
