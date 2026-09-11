<script>
  import Icon from './Icon.svelte';
  import { PLATFORM_LABEL } from '../format.js';
  let { platform, dim = false, class: cls = 'h-4 w-4', label = false } = $props();
  const icon = { ios: 'apple', macos: 'apple', android: 'android', windows: 'windows' }[platform] || 'info';
</script>

<span class="inline-flex items-center gap-1 {dim ? 'opacity-30' : ''}" title={PLATFORM_LABEL[platform] || platform}>
  <Icon name={icon} class={cls} />
  {#if label}<span class="text-xs">{PLATFORM_LABEL[platform] || platform}</span>{/if}
</span>
