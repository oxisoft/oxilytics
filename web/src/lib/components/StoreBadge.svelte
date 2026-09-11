<script>
  import { STORE_LABEL } from '../format.js';
  let { store } = $props();
  const cls = { appstore: 'bg-zinc-800 text-white dark:bg-zinc-200 dark:text-zinc-900', googleplay: 'bg-emerald-600 text-white', msstore: 'bg-sky-600 text-white' }[store] || 'bg-zinc-200';
</script>

<span class="badge {cls}">{STORE_LABEL[store] || store}</span>
