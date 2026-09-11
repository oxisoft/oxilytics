<script>
  import EmptyState from '../lib/components/EmptyState.svelte';
  let { params = {} } = $props();
</script>

<h1 class="mb-4 text-lg font-semibold">Sync run</h1>
<EmptyState icon="sync" title="Coming in a later milestone" message="Sync run is not implemented yet." />
