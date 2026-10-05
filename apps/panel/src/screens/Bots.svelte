<script lang="ts">
  import { api } from '../lib/api';
  import { labels } from '../lib/labels';
  import type { Bot } from '../lib/types';

  interface Props {
    revision?: number
    notify?: (message: string, error?: boolean) => void
  }
  let { revision = 0, notify }: Props = $props()

  let bots = $state<Bot[]>([])
  let name = $state('')
  let numbers = $state('')
  let busy = $state(false)
  let selected = $state('')

  let editName = $state('')
  let editEnabled = $state(true)
  let editNumbers = $state('')
  let editBusy = $state(false)

  const selectedBot = $derived(bots.find((bot) => bot.id === selected) ?? bots[0] ?? null)

  async function load () {
    try {
      bots = (await api<{ bots: Bot[] }>('/api/panel/bots')).bots
    } catch {
      bots = []
    }
  }

  $effect(() => {
    void revision
    void load()
  })

  $effect(() => {
    const bot = selectedBot
    if (!bot) return
    editName = bot.name
    editEnabled = bot.enabled
    editNumbers = (bot.allowlist ?? []).join(', ')
  })

  function parseNumbers (value: string) {
    return value.split(',').map((item) => item.replace(/[^0-9]/g, '')).filter(Boolean)
  }

  function statusText (status: string) {
    return labels.whatsappStatus[status] ?? status
  }

  function statusClass (status: string) {
    return status === 'connected' ? 'herb' : status === 'off' || status === 'disabled' ? 'neutral' : 'honey'
  }

  async function add () {
    if (!name.trim()) return
    busy = true
    try {
      await api('/api/panel/bots', { method: 'POST', body: JSON.stringify({ name: name.trim(), allowlist: parseNumbers(numbers), enabled: true }) })
      name = ''
      numbers = ''
      await load()
    } catch (error) {
      notify?.((error as Error).message, true)
    } finally {
      busy = false
    }
  }

  async function save () {
    if (!selectedBot) return
    editBusy = true
    try {
      await api(`/api/panel/bots/${selectedBot.id}`, {
        method: 'PATCH',
        body: JSON.stringify({ name: editName.trim() || selectedBot.name, enabled: editEnabled, allowlist: parseNumbers(editNumbers) }),
      })
      notify?.(labels.bots.saved)
      await load()
    } catch (error) {
      notify?.((error as Error).message, true)
    } finally {
      editBusy = false
    }
  }

  async function retry () {
    if (!selectedBot) return
    try {
      await api(`/api/panel/bots/${selectedBot.id}/pairing/retry`, { method: 'POST' })
      await load()
    } catch (error) {
      notify?.((error as Error).message, true)
    }
  }

  async function logout () {
    if (!selectedBot || !window.confirm(labels.bots.logoutConfirm)) return
    try {
      await api(`/api/panel/bots/${selectedBot.id}/logout`, { method: 'POST' })
      await load()
    } catch (error) {
      notify?.((error as Error).message, true)
    }
  }

  async function remove (id: string) {
    if (!window.confirm(labels.bots.removeConfirm)) return
    try {
      await api(`/api/panel/bots/${id}`, { method: 'DELETE' })
      await load()
    } catch (error) {
      notify?.((error as Error).message, true)
    }
  }
</script>

<section class="screen">
  <div class="note" style="margin-bottom:20px">
    <svg viewBox="0 0 24 24"><path d="M12 8v5M12 16h.01" /><circle cx="12" cy="12" r="9" /></svg>
    <span>{labels.bots.preview}</span>
  </div>

  <div class="screen-head">
    <div class="actions add-form">
      <input class="input" bind:value={name} placeholder={labels.bots.name} />
      <input class="input" bind:value={numbers} placeholder={labels.bots.numbers} data-wide />
      <button class="btn btn-primary" onclick={add} disabled={busy || !name.trim()}>{labels.bots.add}</button>
    </div>
  </div>

  <div class="with-drawer">
    <div class="panel">
      <table class="table">
        <thead>
          <tr>
            <th>{labels.bots.account}</th>
            <th>{labels.bots.status}</th>
            <th>{labels.bots.allowlist}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each bots as bot (bot.id)}
            <tr onclick={() => (selected = bot.id)} style="cursor:pointer" class:active={selectedBot?.id === bot.id}>
              <td class="cell-acct" data-label={labels.bots.account}>
                <b>{bot.name}</b>
                <span>{bot.phone || (bot.status === 'connected' ? labels.bots.linked : labels.bots.noNumber)}</span>
              </td>
              <td data-label={labels.bots.status}><span class="stamp {statusClass(bot.status)}">{statusText(bot.status)}</span></td>
              <td data-label={labels.bots.allowlist}>
                {#if bot.allowlist && bot.allowlist.length > 0}
                  <div class="chips">
                    {#each bot.allowlist as number (number)}<span class="chip mono">{number}</span>{/each}
                  </div>
                {:else}
                  <span class="muted small">{labels.bots.noAllowlist}</span>
                {/if}
              </td>
              <td class="acc"><button class="btn btn-danger btn-sm" onclick={() => remove(bot.id)}>{labels.bots.remove}</button></td>
            </tr>
          {/each}
          {#if bots.length === 0}
            <tr><td colspan="4" class="muted">{labels.destinos.empty}</td></tr>
          {/if}
        </tbody>
      </table>
    </div>

    <aside class="drawer">
      {#if selectedBot}
        <div class="drawer-head">
          <h3>{labels.bots.edit}</h3>
        </div>

        {#if selectedBot.status === 'unlinked' || selectedBot.status === 'connecting'}
          <div class="qr-wrap">
            <img src={`/api/panel/bots/${selectedBot.id}/qr.png?v=${revision}`} alt="Código QR" style="width:200px;height:200px;background:#fff;padding:8px;border-radius:8px" />
            <div class="qr-cap">{labels.bots.scan}</div>
          </div>
        {/if}

        <div class="field">
          <label class="label" for="bot-name">{labels.bots.name}</label>
          <input class="input" id="bot-name" bind:value={editName} />
        </div>
        <div class="field">
          <label class="label" for="bot-numbers">{labels.bots.allowlist}</label>
          <input class="input" id="bot-numbers" bind:value={editNumbers} placeholder="+57 3…, +57 3…" />
          <p class="helper">{labels.bots.numbersHint}</p>
        </div>
        <div class="field">
          <label class="check"><input type="checkbox" bind:checked={editEnabled} /> {labels.bots.enabled}</label>
        </div>

        <div class="drawer-foot">
          {#if selectedBot.status === 'unlinked' || selectedBot.status === 'connecting'}
            <button class="btn btn-kraft btn-sm" onclick={retry}>{labels.bots.retry}</button>
          {:else}
            <button class="btn btn-ghost btn-sm" onclick={logout}>{labels.bots.logout}</button>
          {/if}
          <button class="btn btn-primary btn-sm" onclick={save} disabled={editBusy}>{labels.bots.save}</button>
        </div>
      {:else}
        <div class="note">
          <svg viewBox="0 0 24 24"><path d="M12 8v5M12 16h.01" /><circle cx="12" cy="12" r="9" /></svg>
          <span>{labels.bots.pairing}</span>
        </div>
      {/if}
    </aside>
  </div>
</section>
