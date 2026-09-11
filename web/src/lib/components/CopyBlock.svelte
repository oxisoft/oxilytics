<script>
  import Icon from './Icon.svelte';
  import { toasts } from '../toast.svelte.js';
  let { text, lang = '' } = $props();
  async function copy() {
    try { await navigator.clipboard.writeText(text); toasts.success('Copied'); } catch { toasts.error('Copy failed'); }
  }
</script>

<div class="relative">
  <pre class="overflow-x-auto rounded-md bg-zinc-900 p-3 text-xs text-zinc-100 dark:bg-black"><code class={lang}>{text}</code></pre>
  <button class="absolute top-2 right-2 rounded bg-zinc-700 p-1 text-zinc-200 hover:bg-zinc-600" onclick={copy} aria-label="Copy"><Icon name="copy" class="h-3.5 w-3.5" /></button>
</div>
