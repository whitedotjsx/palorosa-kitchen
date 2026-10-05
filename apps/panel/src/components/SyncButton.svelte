<script lang="ts">
  import { onMount } from 'svelte'
  import { labels } from '../lib/labels'
  import { api } from '../lib/api'

  interface Props {
    revision?: number
    /** Day on screen; synced together with today and tomorrow. */
    date?: string | undefined
    notify?: ((message: string, error?: boolean) => void) | undefined
    onsynced?: (() => void) | undefined
  }
  let { revision = 0, date, notify, onsynced }: Props = $props()

  interface SyncStatus {
    enabled: boolean
    running: boolean
    intervalMinutes: number
    at?: string
    error?: string
    result?: { kept: number, added: number, removed: number, perDate?: Record<string, number> }
  }

  let status = $state<SyncStatus | null>(null)
  let busy = $state(false)
  let now = $state(Date.now())

  async function loadStatus () {
    try {
      status = await api<SyncStatus>('/api/panel/sync')
    } catch {
      status = null
    }
  }

  async function sync () {
    if (busy) return
    busy = true
    try {
      status = await api<SyncStatus>('/api/panel/sync', { method: 'POST', body: JSON.stringify({ date }) })
      const count = date ? status.result?.perDate?.[date] : undefined
      notify?.(labels.sync.done.replace('{n}', String(count ?? status.result?.kept ?? 0)))
    } catch (error) {
      notify?.((error as Error).message, true)
      await loadStatus()
    } finally {
      busy = false
      onsynced?.()
    }
  }

  function ago (at?: string) {
    if (!at) return labels.sync.never
    const minutes = Math.floor((now - Date.parse(at)) / 60000)
    if (minutes < 1) return labels.sync.justNow
    if (minutes < 60) return labels.sync.minutes.replace('{n}', String(minutes))
    return new Date(at).toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit' })
  }

  $effect(() => {
    void revision
    void loadStatus()
  })

  onMount(() => {
    const timer = setInterval(() => (now = Date.now()), 30000)
    return () => clearInterval(timer)
  })
</script>

{#if status?.enabled}
  <div class="sync">
    <button class="btn btn-kraft btn-sm" onclick={sync} disabled={busy || status.running} title={labels.sync.hint}>
      <svg class:spin={busy || status.running} viewBox="0 0 24 24"><path d="M20 11a8 8 0 0 0-14.6-4.5M4 4v4h4" /><path d="M4 13a8 8 0 0 0 14.6 4.5M20 20v-4h-4" /></svg>
      {busy || status.running ? labels.sync.running : labels.sync.button}
    </button>
    <span class="sync-meta" class:err={!!status.error} title={status.error ?? ''}>
      {status.error ? labels.sync.failed : `${labels.sync.last} ${ago(status.at)}`}
      {#if !status.error && status.intervalMinutes > 0}· {labels.sync.every.replace('{n}', String(status.intervalMinutes))}{/if}
    </span>
  </div>
{/if}

<style>
  .sync {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    min-width: 0;
  }
  .sync-meta {
    font-size: 12px;
    color: var(--text-muted);
  }
  .sync-meta.err {
    color: var(--danger);
    cursor: help;
  }
  .spin {
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
