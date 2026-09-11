<script>
  import { session } from '../session.svelte.js';
  import { fmtDateTime } from '../format.js';
  let { syncStatus = null } = $props();
</script>

<footer class="border-t border-zinc-200 px-4 py-3 text-xs text-zinc-500 dark:border-zinc-800">
  <div class="mx-auto flex max-w-7xl flex-wrap items-center gap-x-4 gap-y-1">
    <span>Oxilytics {session.version?.version || ''}{#if session.version?.commit && session.version.commit !== 'none'} ({session.version.commit.slice(0, 7)}){/if}</span>
    {#if session.version?.build_time && session.version.build_time !== 'unknown'}<span>built {fmtDateTime(session.version.build_time)}</span>{/if}
    <a class="ml-auto hover:underline" href="https://github.com/oxisoft/oxilytics" target="_blank" rel="noreferrer">GitHub</a>
    <span>MIT</span>
  </div>
</footer>
