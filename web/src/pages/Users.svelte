<script>
  import { onMount } from 'svelte';
  import { api, ApiError } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { fmtDateTime } from '../lib/format.js';
  import Modal from '../lib/components/Modal.svelte';
  import ConfirmDialog from '../lib/components/ConfirmDialog.svelte';
  import Icon from '../lib/components/Icon.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import SettingsNav from '../lib/components/SettingsNav.svelte';

  let users = $state(null);
  let error = $state('');
  let editOpen = $state(false);
  let editing = $state(null); // null = create
  let form = $state({ email: '', name: '', role: 'viewer', password: '', disabled: false });
  let fields = $state({});
  let pwOpen = $state(false);
  let pwTarget = $state(null);
  let newPw = $state('');
  let delOpen = $state(false);
  let delTarget = $state(null);

  async function load() {
    try { users = await api.get('/users/'); } catch (e) { error = e.message; }
  }
  onMount(load);

  function openCreate() { editing = null; form = { email: '', name: '', role: 'viewer', password: '', disabled: false }; fields = {}; editOpen = true; }
  function openEdit(u) { editing = u; form = { email: u.email, name: u.name, role: u.role, password: '', disabled: u.disabled }; fields = {}; editOpen = true; }

  async function save(e) {
    e.preventDefault();
    fields = {};
    try {
      if (editing) await api.put('/users/' + editing.id, { name: form.name, role: form.role, disabled: form.disabled });
      else await api.post('/users/', form);
      editOpen = false;
      toasts.success(editing ? 'User updated' : 'User created');
      load();
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length) fields = err.fields;
      else toasts.error(err.message);
    }
  }

  async function resetPw() {
    try { await api.put('/users/' + pwTarget.id + '/password', { password: newPw }); pwOpen = false; newPw = ''; toasts.success('Password reset'); }
    catch (err) { toasts.error(err.fields?.password || err.message); }
  }

  async function del() {
    await api.del('/users/' + delTarget.id);
    toasts.success('User deleted');
    load();
  }
</script>

<SettingsNav />
<div class="mb-4 flex items-center justify-between">
  <h1 class="text-lg font-semibold">Users</h1>
  <button class="btn-primary" onclick={openCreate}><Icon name="plus" />Add user</button>
</div>

{#if error}<p class="text-red-600">{error}</p>
{:else if !users}<Skeleton rows={4} />
{:else}
  <div class="card overflow-x-auto p-0">
    <table class="table">
      <thead><tr><th>Name</th><th>E-mail</th><th>Role</th><th>2FA</th><th>Status</th><th>Last login</th><th></th></tr></thead>
      <tbody>
        {#each users as u (u.id)}
          <tr>
            <td class="font-medium">{u.name}{#if u.id === session.user?.id}<span class="ml-1 text-xs text-zinc-400">(you)</span>{/if}</td>
            <td>{u.email}</td>
            <td><span class="badge {u.role === 'admin' ? 'bg-brand-50 text-brand-700 dark:bg-brand-700/30 dark:text-brand-50' : 'bg-zinc-100 dark:bg-zinc-800'}">{u.role}</span></td>
            <td>{u.totp_enabled ? 'on' : '—'}</td>
            <td>{#if u.disabled}<span class="badge bg-red-100 text-red-700">disabled</span>{:else}<span class="badge bg-emerald-100 text-emerald-700">active</span>{/if}</td>
            <td class="text-zinc-500">{fmtDateTime(u.last_login_at)}</td>
            <td class="text-right whitespace-nowrap">
              <button class="rounded p-1 hover:bg-zinc-200 dark:hover:bg-zinc-700" title="Edit" onclick={() => openEdit(u)}><Icon name="edit" /></button>
              <button class="rounded p-1 hover:bg-zinc-200 dark:hover:bg-zinc-700" title="Reset password" onclick={() => { pwTarget = u; newPw = ''; pwOpen = true; }}><Icon name="refresh" /></button>
              <button class="rounded p-1 text-red-600 hover:bg-red-50 dark:hover:bg-red-950" title="Delete" disabled={u.id === session.user?.id} onclick={() => { delTarget = u; delOpen = true; }}><Icon name="trash" /></button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

<Modal bind:open={editOpen} title={editing ? 'Edit user' : 'Add user'}>
  <form class="space-y-3" onsubmit={save}>
    <div><label class="label" for="u-email">E-mail</label><input id="u-email" class="input" type="email" bind:value={form.email} disabled={!!editing} required />{#if fields.email}<p class="text-xs text-red-600">{fields.email}</p>{/if}</div>
    <div><label class="label" for="u-name">Name</label><input id="u-name" class="input" bind:value={form.name} required />{#if fields.name}<p class="text-xs text-red-600">{fields.name}</p>{/if}</div>
    <div><label class="label" for="u-role">Role</label><select id="u-role" class="input" bind:value={form.role}><option value="viewer">viewer — read only</option><option value="admin">admin — full access</option></select>{#if fields.role}<p class="text-xs text-red-600">{fields.role}</p>{/if}</div>
    {#if !editing}
      <div><label class="label" for="u-pw">Temporary password (min 10 chars)</label><input id="u-pw" class="input" type="text" bind:value={form.password} required minlength="10" autocomplete="off" />{#if fields.password}<p class="text-xs text-red-600">{fields.password}</p>{/if}</div>
    {:else}
      <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={form.disabled} />Disabled (cannot sign in)</label>
    {/if}
    <div class="flex justify-end gap-2"><button type="button" class="btn-secondary" onclick={() => (editOpen = false)}>Cancel</button><button class="btn-primary">{editing ? 'Save' : 'Create'}</button></div>
  </form>
</Modal>

<Modal bind:open={pwOpen} title="Reset password for {pwTarget?.email}">
  <label class="label" for="npw">New password (min 10 chars)</label>
  <input id="npw" class="input" type="text" bind:value={newPw} minlength="10" autocomplete="off" />
  <div class="mt-4 flex justify-end gap-2"><button class="btn-secondary" onclick={() => (pwOpen = false)}>Cancel</button><button class="btn-primary" onclick={resetPw} disabled={newPw.length < 10}>Reset</button></div>
</Modal>

<ConfirmDialog bind:open={delOpen} title="Delete user" message="Delete {delTarget?.email}? This cannot be undone." confirmLabel="Delete" danger onconfirm={del} />
