<script>
  import { link } from 'svelte-spa-router';
  import EmptyState from '../lib/components/EmptyState.svelte';
</script>

<EmptyState icon="alert" title="Page not found" message="The address is wrong or the page moved.">
  <a href="/" use:link class="btn-primary">Go to dashboard</a>
</EmptyState>
