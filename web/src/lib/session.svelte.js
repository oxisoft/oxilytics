// Global session state (Svelte 5 runes in a .svelte.js module).
class Session {
  user = $state(null);
  loaded = $state(false);
  setupRequired = $state(false);
  version = $state(null);
  setup = $state(null);

  get isAdmin() { return this.user?.role === 'admin'; }
  get authenticated() { return !!this.user; }

  set(user) { this.user = user; this.loaded = true; }
  clear() { this.user = null; this.loaded = true; }
}

export const session = new Session();
