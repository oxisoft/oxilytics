<script>
  import { session } from '../lib/session.svelte.js';
  import { fmtDateTime } from '../lib/format.js';
  import Icon from '../lib/components/Icon.svelte';
  const v = $derived(session.version || {});
</script>

<div class="mx-auto max-w-lg">
  <div class="mb-6 flex items-center gap-3">
    <span class="grid h-12 w-12 place-items-center rounded-xl bg-brand-600 text-white"><Icon name="dashboard" class="h-6 w-6" /></span>
    <div>
      <h1 class="text-xl font-semibold">OxiLytics</h1>
      <p class="text-sm text-zinc-500">Self-hosted app store analytics</p>
    </div>
  </div>

  <div class="card">
    <h2 class="mb-3 font-medium">Build</h2>
    <dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
      <dt class="text-zinc-500">Version</dt><dd class="font-mono">{v.version || '—'}</dd>
      <dt class="text-zinc-500">Commit</dt>
      <dd class="font-mono">
        {#if v.commit && v.commit !== 'none'}
          <a class="hover:underline" href="https://github.com/oxisoft/oxilytics/commit/{v.commit.replace('-dirty', '')}" target="_blank" rel="noreferrer">{v.commit}</a>
        {:else}—{/if}
      </dd>
      <dt class="text-zinc-500">Built</dt><dd>{v.build_time && v.build_time !== 'unknown' ? fmtDateTime(v.build_time) : '—'}</dd>
      <dt class="text-zinc-500">Go</dt><dd class="font-mono">{v.go || '—'}</dd>
    </dl>
  </div>

  <div class="card mt-4 text-sm">
    <h2 class="mb-2 font-medium">Links</h2>
    <ul class="space-y-1">
      <li><a class="flex items-center gap-2 hover:underline" href="https://oxisoft.io" target="_blank" rel="noreferrer"><Icon name="external" />Developed by OxiSoft</a></li>
      <li><a class="flex items-center gap-2 hover:underline" href="https://github.com/oxisoft/oxilytics" target="_blank" rel="noreferrer"><Icon name="external" />Source on GitHub</a></li>
      <li><a class="flex items-center gap-2 hover:underline" href="https://github.com/oxisoft/oxilytics/releases" target="_blank" rel="noreferrer"><Icon name="external" />Releases &amp; changelog</a></li>
      <li><a class="flex items-center gap-2 hover:underline" href="https://github.com/oxisoft/oxilytics/issues" target="_blank" rel="noreferrer"><Icon name="external" />Report an issue</a></li>
    </ul>
    <p class="mt-3 text-xs text-zinc-500">MIT licensed. Reads only the App Store Connect and Google Play APIs — no telemetry, no tracking SDK.</p>
  </div>
</div>
