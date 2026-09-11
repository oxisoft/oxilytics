<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import StoreCard from '../lib/components/StoreCard.svelte';
  import CopyBlock from '../lib/components/CopyBlock.svelte';
  import Banner from '../lib/components/Banner.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';

  let { embedded = false } = $props();
  let status = $state(null);

  onMount(async () => { status = await api.get('/setup/status'); session.setup = status; });

  const envSnippet = `# .env next to docker-compose.yml
OXI_ASC_KEY_ID=
OXI_ASC_ISSUER_ID=
OXI_GPLAY_BUCKET=

# files in ./secrets/ (mounted read-only into the container)
#   ./secrets/AuthKey.p8      → OXI_ASC_KEY_FILE=/secrets/AuthKey.p8
#   ./secrets/gplay-sa.json   → OXI_GPLAY_SA_FILE=/secrets/gplay-sa.json`;
</script>

{#if !embedded}
  <h1 class="mb-1 text-lg font-semibold">Setup</h1>
{/if}

{#if status?.setup_required}
  <div class="mb-4">
    <Banner kind="warn" message="Oxilytics needs access to at least one store. Configure App Store Connect or Google Play below, restart the container, then run a full sync." />
  </div>
{:else if status}
  <p class="mb-4 text-sm text-zinc-500">Credentials live in environment variables and mounted files, never in the database. Add a second store any time.</p>
{/if}

{#if !status}<Skeleton rows={5} />
{:else}
  <div class="grid gap-4 lg:grid-cols-2">
    <StoreCard store="appstore" status={status.stores.appstore} />
    <StoreCard store="googleplay" status={status.stores.googleplay} />
  </div>

  <details class="card mt-4">
    <summary class="cursor-pointer text-sm font-medium">Where do these values go?</summary>
    <div class="mt-3"><CopyBlock text={envSnippet} /></div>
    {#if !session.isAdmin}<p class="mt-2 text-xs text-zinc-500">Ask an administrator to configure the container.</p>{/if}
  </details>
{/if}
