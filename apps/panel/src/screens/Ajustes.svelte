<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { labels } from '../lib/labels'
  import type { CheckResult, Diagnostics, Invite, Notifications, PanelState, SessionRow, Settings, TunnelInfo, UpdateState } from '../lib/types'

  interface Props {
    notify: (message: string, error?: boolean) => void
    tunnel: TunnelInfo | null
    reloadTunnel: () => Promise<void>
  }
  let { notify, tunnel, reloadTunnel }: Props = $props()

  let settings = $state<Settings | null>(null)
  let invites = $state<Invite[]>([])
  let sessions = $state<SessionRow[]>([])
  let busy = $state(false)
  let inviteLabel = $state('')
  let inviteLink = $state('')

  let notifications = $state<Notifications | null>(null)
  let autostart = $state(false)
  let diagnostics = $state<Diagnostics | null>(null)
  let update = $state<UpdateState | null>(null)
  let updateBusy = $state(false)
  const updateStatusText = $derived.by(() => {
    if (!update) return labels.ajustes.updateIdle
    switch (update.status) {
      case 'checking': return labels.ajustes.updateChecking
      case 'downloading':
      case 'applying': return labels.ajustes.updateApplying
      case 'updated': return labels.ajustes.updateInstalled(update.latest ?? update.current)
      case 'error': return labels.ajustes.updateError(update.error ?? '')
      case 'available': return labels.ajustes.updateAvailable(update.latest ?? '')
      default: return labels.ajustes.updateUpToDate(update.current)
    }
  })
  let advanced = $state({ catalogPath: '', panelPort: 5211, debug: false, syncMinutes: 10 })
  let notifBusy = $state(false)
  let advancedBusy = $state(false)
  let checks = $state<CheckResult[]>([])
  let checksRunning = $state<Set<string>>(new Set())
  let checksAll = $state(false)
  const checkCounts = $derived({
    ok: checks.filter((c) => c.status === 'ok').length,
    warn: checks.filter((c) => c.status === 'warn').length,
    fail: checks.filter((c) => c.status === 'fail').length,
  })

  const notifRows: { key: keyof Notifications; label: string }[] = [
    { key: 'enabled', label: labels.ajustes.notifEnabled },
    { key: 'new', label: labels.ajustes.notifNew },
    { key: 'updated', label: labels.ajustes.notifUpdated },
    { key: 'today', label: labels.ajustes.notifToday },
    { key: 'tomorrow', label: labels.ajustes.notifTomorrow },
  ]

  // Secret fields are write only: the API returns them masked, so a new value
  // is sent only when the operator types one.
  let secrets = $state({
    adminPassword: '',
    consumerSecret: '',
    exportCronKey: '',
    webhookSecret: '',
  })

  async function loadSettings () {
    try {
      settings = await api<Settings>('/api/panel/settings')
      advanced = {
        catalogPath: settings.catalogPath ?? '',
        panelPort: settings.panelPort || 5211,
        syncMinutes: settings.syncMinutes ?? 10,
        debug: !!settings.debug,
      }
    } catch (error) {
      notify((error as Error).message, true)
    }
  }

  async function loadNotifications () {
    try {
      notifications = (await api<PanelState>('/api/panel/state')).snapshot.notifications
    } catch {
      notifications = null
    }
  }

  async function loadAutostart () {
    try {
      autostart = (await api<{ enabled: boolean }>('/api/panel/autostart')).enabled
    } catch {
      autostart = false
    }
  }

  async function loadDiagnostics () {
    try {
      diagnostics = (await api<{ diagnostics: Diagnostics }>('/api/panel/diagnostics')).diagnostics
    } catch {
      diagnostics = null
    }
  }

  async function loadChecks () {
    try {
      checks = (await api<{ checks: CheckResult[] }>('/api/panel/selftest')).checks
    } catch {
      checks = []
    }
  }

  async function loadUpdate () {
    try {
      update = (await api<{ update: UpdateState }>('/api/panel/update')).update
    } catch {
      update = null
    }
  }

  async function checkUpdate () {
    updateBusy = true
    try {
      update = (await api<{ update: UpdateState }>('/api/panel/update/check', { method: 'POST' })).update
      if (update.available) {
        notify(labels.ajustes.updateAvailable(update.latest ?? ''))
      } else {
        notify(labels.ajustes.updateUpToDate(update.current))
      }
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      updateBusy = false
    }
  }

  async function applyUpdate () {
    updateBusy = true
    try {
      update = (await api<{ update: UpdateState }>('/api/panel/update/apply', { method: 'POST' })).update
      notify(labels.ajustes.updateApplying)
      await pollUpdate()
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      updateBusy = false
    }
  }

  // The app restarts when the replacement is ready; the poll usually ends
  // with a dropped request as the server goes down.
  async function pollUpdate () {
    for (let attempt = 0; attempt < 120; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 2000))
      try {
        update = (await api<{ update: UpdateState }>('/api/panel/update')).update
        if (update.status === 'updated' || update.status === 'error') return
      } catch {
        return
      }
    }
  }

  async function runAllChecks () {
    checksAll = true
    checksRunning = new Set(checks.map((c) => c.id))
    try {
      checks = (await api<{ checks: CheckResult[] }>('/api/panel/selftest', { method: 'POST' })).checks
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      checksAll = false
      checksRunning = new Set()
    }
  }

  async function runCheck (id: string) {
    checksRunning = new Set([...checksRunning, id])
    try {
      const { check } = await api<{ check: CheckResult }>(`/api/panel/selftest/${id}`, { method: 'POST' })
      checks = checks.map((c) => (c.id === id ? check : c))
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      const next = new Set(checksRunning)
      next.delete(id)
      checksRunning = next
    }
  }

  function checkStamp (status: string): string {
    return { ok: 'herb', warn: 'honey', fail: 'jam' }[status] ?? 'neutral'
  }

  async function loadAuth () {
    try {
      invites = (await api<{ invites: Invite[] }>('/api/panel/invites')).invites
      sessions = (await api<{ sessions: SessionRow[] }>('/api/panel/sessions')).sessions
    } catch (error) {
      notify((error as Error).message, true)
    }
  }

  onMount(async () => {
    await Promise.all([loadSettings(), loadAuth(), loadNotifications(), loadAutostart(), loadDiagnostics(), loadChecks(), loadUpdate()])
  })

  async function setNotification (key: keyof Notifications) {
    if (!notifications) return
    const next = { ...notifications, [key]: !notifications[key] }
    notifBusy = true
    try {
      await api('/api/panel/notifications', { method: 'PATCH', body: JSON.stringify(next) })
      notifications = next
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      notifBusy = false
    }
  }

  async function toggleAutostart () {
    try {
      const result = await api<{ enabled: boolean }>('/api/panel/autostart', { method: 'POST', body: JSON.stringify({ enabled: !autostart }) })
      autostart = result.enabled
    } catch (error) {
      notify((error as Error).message, true)
    }
  }

  async function saveAdvanced () {
    advancedBusy = true
    try {
      settings = await api<Settings>('/api/panel/settings', {
        method: 'PATCH',
        body: JSON.stringify({ catalogPath: advanced.catalogPath, panelPort: Number(advanced.panelPort) || 5211, debug: advanced.debug, syncMinutes: Math.max(0, Math.min(1440, Math.round(Number(advanced.syncMinutes) || 0))) }),
      })
      notify(labels.ajustes.savedAdvanced)
      await loadDiagnostics()
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      advancedBusy = false
    }
  }

  let showLogs = $state(false)
  let logsBusy = $state(false)

  async function exportLogs () {
    logsBusy = true
    try {
      const response = await fetch('/api/panel/logs/download', { credentials: 'same-origin' })
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as { error?: string }
        throw new Error(body.error ?? `HTTP ${response.status}`)
      }
      const blob = await response.blob()
      const disposition = response.headers.get('content-disposition') ?? ''
      const name = /filename="([^"]+)"/.exec(disposition)?.[1] ?? 'palorosa-registros.log'
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = name
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
      notify(labels.ajustes.logsExported)
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      logsBusy = false
    }
  }

  let configBusy = $state(false)
  let configInput: HTMLInputElement | undefined = $state()

  async function exportConfig () {
    configBusy = true
    try {
      const response = await fetch('/api/panel/settings/export', { credentials: 'same-origin' })
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as { error?: string }
        throw new Error(body.error ?? `HTTP ${response.status}`)
      }
      const blob = await response.blob()
      const disposition = response.headers.get('content-disposition') ?? ''
      const name = /filename="([^"]+)"/.exec(disposition)?.[1] ?? 'palorosa-configuracion.json'
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = name
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
      notify(labels.ajustes.configExported)
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      configBusy = false
    }
  }

  async function importConfig (event: Event) {
    const input = event.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    input.value = ''
    if (!file || !confirm(labels.ajustes.importConfirm)) return
    configBusy = true
    try {
      settings = await api<Settings>('/api/panel/settings/import', { method: 'POST', body: await file.text() })
      secrets = { adminPassword: '', consumerSecret: '', exportCronKey: '', webhookSecret: '' }
      notify(labels.ajustes.configImported)
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      configBusy = false
    }
  }

  async function openLogFolder () {
    logsBusy = true
    try {
      await api('/api/panel/logs/open', { method: 'POST' })
      notify(labels.ajustes.logsFolderOpened)
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      logsBusy = false
    }
  }

  async function copyLogs () {
    try {
      await navigator.clipboard.writeText(diagnostics?.logs ?? '')
      notify(labels.ajustes.logsCopied)
    } catch {
      notify('No pudimos copiar los registros', true)
    }
  }

  async function save () {
    if (!settings) return false
    busy = true
    try {
      const wp: Record<string, string> = {
        adminUrl: settings.wp.adminUrl,
        adminUser: settings.wp.adminUser,
        consumerKey: settings.wp.consumerKey,
        exportId: settings.wp.exportId,
      }
      if (secrets.adminPassword) wp.adminPassword = secrets.adminPassword
      if (secrets.consumerSecret) wp.consumerSecret = secrets.consumerSecret
      if (secrets.exportCronKey) wp.exportCronKey = secrets.exportCronKey
      const body: Record<string, unknown> = {
        domain: settings.domain,
        tunnelHostname: settings.tunnelHostname,
        access: { enabled: settings.access.enabled },
        wp,
      }
      if (secrets.webhookSecret) body.webhookSecret = secrets.webhookSecret
      settings = await api<Settings>('/api/panel/settings', { method: 'PATCH', body: JSON.stringify(body) })
      secrets = { adminPassword: '', consumerSecret: '', exportCronKey: '', webhookSecret: '' }
      notify(labels.ajustes.saved)
      return true
    } catch (error) {
      notify((error as Error).message, true)
      return false
    } finally {
      busy = false
    }
  }

  // Saves and asks the desktop app to relaunch itself, so settings that only
  // apply on start (tunnel, port, catalog) take effect without closing by hand.
  let restarting = $state(false)

  async function saveAndRestart () {
    if (!(await save())) return
    restarting = true
    try {
      await api('/api/panel/restart', { method: 'POST' })
      notify(labels.ajustes.restarting)
    } catch (error) {
      notify((error as Error).message, true)
      restarting = false
    }
  }

  // Empty wp-admin URL: the server infers it from the store domain.
  const inferredSite = $derived(
    `https://${(settings?.domain || 'palorosabreakfast.com').trim().replace(/^https?:\/\//, '').replace(/\/+$/, '')}`
  )

  let loginTesting = $state(false)

  // Save first so the check uses what is typed now, then run only the login check.
  async function testLogin () {
    loginTesting = true
    try {
      await save()
      const { check } = await api<{ check: CheckResult }>('/api/panel/selftest/wp_login', { method: 'POST' })
      checks = checks.map((c) => (c.id === check.id ? check : c))
      notify(`${labels.ajustes.loginResult[check.status as 'ok' | 'warn' | 'fail' | 'skipped'] ?? check.status}: ${check.detail}`, check.status !== 'ok')
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      loginTesting = false
    }
  }

  // Copies text even where the async clipboard API is blocked (old WebView,
  // plain http): falls back to a hidden textarea + execCommand.
  async function copyText (text: string, done: string) {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      const area = document.createElement('textarea')
      area.value = text
      area.setAttribute('readonly', '')
      area.style.cssText = 'position:fixed;opacity:0;pointer-events:none'
      document.body.appendChild(area)
      area.select()
      const ok = document.execCommand('copy')
      area.remove()
      if (!ok) {
        notify(labels.ajustes.copyFailed, true)
        return
      }
    }
    notify(done)
  }

  const inviteCode = $derived.by(() => {
    try {
      return new URL(inviteLink).searchParams.get('invite') ?? ''
    } catch {
      return ''
    }
  })

  async function createInvite () {
    busy = true
    try {
      const result = await api<{ url: string }>('/api/panel/invites', {
        method: 'POST',
        body: JSON.stringify({ label: inviteLabel }),
      })
      inviteLink = result.url
      inviteLabel = ''
      await loadAuth()
    } catch (error) {
      notify((error as Error).message, true)
    } finally {
      busy = false
    }
  }

  async function revokeInvite (id: string) {
    try {
      await api(`/api/panel/invites/${id}`, { method: 'DELETE' })
      await loadAuth()
    } catch (error) {
      notify((error as Error).message, true)
    }
  }

  async function revokeSession (id: string) {
    try {
      await api(`/api/panel/sessions/${id}`, { method: 'DELETE' })
      await loadAuth()
    } catch (error) {
      notify((error as Error).message, true)
    }
  }

  async function loginTunnel () {
    try {
      await api('/api/panel/tunnel/login', { method: 'POST' })
      notify(labels.ajustes.tunnelLoginStarted)
      window.setTimeout(() => void reloadTunnel(), 2500)
    } catch (error) {
      notify((error as Error).message, true)
    }
  }

  async function copyUrl () {
    if (!tunnel?.url) return
    try {
      await navigator.clipboard.writeText(tunnel.url)
      notify('URL copiada')
    } catch {
      notify('No pudimos copiar la URL', true)
    }
  }

  const statusText = $derived(tunnel ? (labels.tunnelStatus[tunnel.status] ?? tunnel.status) : '…')
  const statusClass = $derived(tunnel?.status === 'running' ? 'herb' : tunnel?.status === 'errored' ? 'jam' : 'honey')

  function formatDate (value: string) {
    if (!value) return '—'
    try {
      return new Date(value).toLocaleString('es-CO', { dateStyle: 'short', timeStyle: 'short' })
    } catch {
      return value
    }
  }
</script>

<div class="stack">
  <div>
    <h3 class="block-title">{labels.ajustes.tunnel}</h3>
    <div class="kraft-panel">
      <div class="kv">
        <span class="k">{labels.ajustes.tunnelHostname}</span>
        <span class="v">{tunnel?.hostname || '—'}</span>
      </div>
      <div class="kv">
        <span class="k">{labels.ajustes.tunnelStatus}</span>
        <span class="v"><span class="stamp {statusClass} big">{statusText}</span></span>
      </div>
      <div class="kv">
        <span class="k">{labels.ajustes.tunnelCert}</span>
        <span class="v">{tunnel?.certPresent ? labels.ajustes.tunnelPresent : labels.ajustes.tunnelMissing}</span>
      </div>
      <div class="actions">
        {#if !tunnel?.certPresent}
          <button class="btn btn-kraft" onclick={loginTunnel}>{labels.ajustes.tunnelLogin}</button>
        {/if}
        <button class="btn btn-ghost" onclick={copyUrl} disabled={!tunnel?.url}>{labels.ajustes.tunnelCopy}</button>
      </div>
    </div>
  </div>

  {#if settings}
    <div>
      <h3 class="block-title">{labels.ajustes.store}</h3>
      <div class="kraft-panel">
        <div class="field-row">
          <div class="field">
            <label class="label" for="domain">{labels.ajustes.domain}</label>
            <input class="input" id="domain" bind:value={settings.domain} placeholder="palorosabreakfast.com" />
          </div>
          <div class="field">
            <label class="label" for="tunnel-hostname">{labels.ajustes.tunnelHostname}</label>
            <input class="input" id="tunnel-hostname" bind:value={settings.tunnelHostname} placeholder="cocina.palorosabreakfast.com" />
          </div>
        </div>
        <div class="field-row">
          <div class="field">
            <label class="label" for="admin-url">{labels.ajustes.adminUrl}</label>
            <input class="input" id="admin-url" bind:value={settings.wp.adminUrl} placeholder={inferredSite} />
            <p class="helper">{labels.ajustes.adminUrlHint}</p>
          </div>
          <div class="field">
            <label class="label" for="admin-user">{labels.ajustes.adminUser}</label>
            <input class="input" id="admin-user" bind:value={settings.wp.adminUser} autocomplete="off" />
          </div>
        </div>
        <div class="field-row">
          <div class="field">
            <label class="label" for="admin-password">{labels.ajustes.adminPassword}</label>
            <input class="input" id="admin-password" type="password" bind:value={secrets.adminPassword} placeholder={settings.wp.adminPassword || labels.ajustes.keepHint} autocomplete="new-password" />
          </div>
          <div class="field">
            <label class="label" for="consumer-key">{labels.ajustes.consumerKey}</label>
            <input class="input" id="consumer-key" bind:value={settings.wp.consumerKey} autocomplete="off" />
          </div>
        </div>
        <div class="field-row">
          <div class="field">
            <label class="label" for="consumer-secret">{labels.ajustes.consumerSecret}</label>
            <input class="input" id="consumer-secret" type="password" bind:value={secrets.consumerSecret} placeholder={settings.wp.consumerSecret || labels.ajustes.keepHint} autocomplete="new-password" />
          </div>
          <div class="field">
            <label class="label" for="export-id">{labels.ajustes.exportId}</label>
            <input class="input" id="export-id" bind:value={settings.wp.exportId} />
          </div>
        </div>
        <div class="field-row">
          <div class="field">
            <label class="label" for="export-cron">{labels.ajustes.exportCronKey}</label>
            <input class="input" id="export-cron" type="password" bind:value={secrets.exportCronKey} placeholder={settings.wp.exportCronKey || labels.ajustes.keepHint} autocomplete="new-password" />
          </div>
          <div class="field">
            <label class="label" for="webhook-secret">{labels.ajustes.webhookSecret}</label>
            <input class="input" id="webhook-secret" type="password" bind:value={secrets.webhookSecret} placeholder={settings.webhookSecret || labels.ajustes.keepHint} autocomplete="new-password" />
          </div>
        </div>
        <p class="helper">{labels.ajustes.keepHint}</p>
        <div class="actions">
          <button class="btn btn-primary" onclick={save} disabled={busy || loginTesting || restarting}>{labels.ajustes.save}</button>
          <button class="btn btn-kraft" onclick={saveAndRestart} disabled={busy || loginTesting || restarting}>
            {restarting ? labels.ajustes.restarting : labels.ajustes.saveRestart}
          </button>
          <button class="btn btn-ghost" onclick={testLogin} disabled={busy || loginTesting || restarting}>
            {loginTesting ? labels.ajustes.loginTesting : labels.ajustes.loginTest}
          </button>
        </div>
      </div>
    </div>

    <div>
      <h3 class="block-title">{labels.ajustes.config}</h3>
      <div class="kraft-panel">
        <p class="helper">{labels.ajustes.configHelp}</p>
        <div class="actions">
          <button class="btn btn-ghost" onclick={exportConfig} disabled={configBusy}>{labels.ajustes.exportConfig}</button>
          <button class="btn btn-ghost" onclick={() => configInput?.click()} disabled={configBusy}>{labels.ajustes.importConfig}</button>
          <input bind:this={configInput} type="file" accept="application/json,.json" hidden onchange={importConfig} />
        </div>
      </div>
    </div>
  {/if}

  {#if notifications}
    <div>
      <h3 class="block-title">{labels.ajustes.notifications}</h3>
      <div class="ruled">
        {#each notifRows as row (row.key)}
          <div class="r">
            <span class="k">{row.label}</span>
            <span class="v">
              <button
                class="switch"
                class:on={notifications[row.key]}
                aria-pressed={notifications[row.key]}
                aria-label={row.label}
                disabled={notifBusy}
                onclick={() => setNotification(row.key)}
              ></button>
            </span>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <div>
    <h3 class="block-title">{labels.ajustes.advanced}</h3>
    <div class="kraft-panel">
      <div class="field-row">
        <div class="field">
          <label class="label" for="catalog-path">{labels.ajustes.catalogPath}</label>
          <input class="input" id="catalog-path" bind:value={advanced.catalogPath} placeholder="data/seed.json" />
        </div>
        <div class="field">
          <label class="label" for="panel-port">{labels.ajustes.panelPort}</label>
          <input class="input" id="panel-port" type="number" min="1" max="65535" bind:value={advanced.panelPort} />
        </div>
      </div>
      <div class="field">
        <label class="label" for="sync-minutes">{labels.ajustes.syncMinutes}</label>
        <input class="input" id="sync-minutes" type="number" min="0" max="1440" bind:value={advanced.syncMinutes} style="max-width:140px" />
        <p class="helper">{labels.ajustes.syncMinutesHint}</p>
      </div>
      <div class="field">
        <label class="check"><input type="checkbox" bind:checked={advanced.debug} /> {labels.ajustes.debug}</label>
      </div>
      <div class="ruled" style="margin:4px 0 14px">
        <div class="r">
          <span class="k">{labels.ajustes.autostart}</span>
          <span class="v">
            <button class="switch" class:on={autostart} aria-pressed={autostart} aria-label={labels.ajustes.autostart} onclick={toggleAutostart}></button>
          </span>
        </div>
      </div>
      <p class="helper">{labels.ajustes.restartHint}</p>
      <div class="actions">
        <button class="btn btn-primary" onclick={saveAdvanced} disabled={advancedBusy}>{labels.ajustes.saveAdvanced}</button>
      </div>
    </div>
  </div>

  {#if checks.length}
    <div>
      <h3 class="block-title">{labels.ajustes.checks}</h3>
      <div class="kraft-panel selftest">
        <p class="helper">{labels.ajustes.checksHint}</p>
        <div class="checks-head">
          <span class="muted small">{labels.ajustes.checksSummary(checkCounts.ok, checkCounts.warn, checkCounts.fail)}</span>
          <button class="btn btn-primary btn-sm" onclick={runAllChecks} disabled={checksAll}>
            {checksAll ? labels.ajustes.checksRunning : labels.ajustes.checksRunAll}
          </button>
        </div>
        <ul class="check-list">
          {#each checks as check (check.id)}
            <li class="check-row" data-status={check.status}>
              <div class="check-main">
                <div class="check-title">
                  <span class="stamp {checkStamp(check.status)}">{checksRunning.has(check.id) ? labels.ajustes.checksRunning : labels.ajustes.checkStatus[check.status ?? '']}</span>
                  <b>{check.label}</b>
                </div>
                <p class="check-detail">{check.detail || labels.ajustes.checksNever}{#if check.checkedAt}<span class="muted">&nbsp;· {check.durationMs} ms</span>{/if}</p>
              </div>
              <button class="btn btn-kraft btn-sm" onclick={() => runCheck(check.id)} disabled={checksRunning.has(check.id)}>{labels.ajustes.checksRun}</button>
            </li>
          {/each}
        </ul>
      </div>
    </div>
  {/if}

  {#if diagnostics}
    <div>
      <h3 class="block-title">{labels.ajustes.diagnostics}</h3>
      <div class="kraft-panel" style="margin-bottom:14px">
        <div class="kv"><span class="k">{labels.ajustes.version}</span><span class="v">{diagnostics.version}</span></div>
        <div class="kv"><span class="k">{labels.ajustes.uptime}</span><span class="v">{diagnostics.uptime}</span></div>
        <div class="kv"><span class="k">{labels.ajustes.dataDir}</span><span class="v">{diagnostics.dataDir}</span></div>
      </div>
      {#if update}
        <div class="kraft-panel" style="margin-bottom:14px">
          <div class="kv"><span class="k">{labels.ajustes.updates}</span><span class="v">{updateStatusText}</span></div>
          <p class="helper">{labels.ajustes.updatesHint}</p>
          <div class="actions">
            <button class="btn btn-kraft" onclick={checkUpdate} disabled={updateBusy}>{labels.ajustes.updateCheck}</button>
            {#if update.available}
              <button class="btn btn-primary" onclick={applyUpdate} disabled={updateBusy}>{labels.ajustes.updateApply}</button>
            {/if}
          </div>
        </div>
      {/if}
      <div class="kraft-panel logs-panel">
        <div class="logs-head">
          <div class="logs-info">
            <b>{labels.ajustes.logs}</b>
            <span class="muted">{labels.ajustes.logsHelp}</span>
            {#if diagnostics.logPath}<span class="logs-path">{diagnostics.logPath}</span>{/if}
          </div>
          <div class="logs-actions">
            <button class="btn btn-ghost btn-sm" aria-expanded={showLogs} onclick={() => (showLogs = !showLogs)}>
              {showLogs ? labels.ajustes.hideLogs : labels.ajustes.showLogs}
            </button>
            <button class="btn btn-kraft btn-sm" onclick={exportLogs} disabled={logsBusy}>{labels.ajustes.exportLogs}</button>
            <button class="btn btn-kraft btn-sm" onclick={openLogFolder} disabled={logsBusy}>{labels.ajustes.openLogFolder}</button>
          </div>
        </div>
        {#if showLogs}
          <div class="logs">
            <pre>{diagnostics.logs || labels.ajustes.logsEmpty}</pre>
            <div class="foot">
              <button class="btn btn-kraft btn-sm" onclick={loadDiagnostics}>{labels.ajustes.refreshLogs}</button>
              <button class="btn btn-kraft btn-sm" onclick={copyLogs}>{labels.ajustes.copyLogs}</button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/if}

  <div>
    <h3 class="block-title">{labels.ajustes.invites}</h3>
    {#if inviteLink}
      <div class="note" style="margin-bottom:14px">
        <svg viewBox="0 0 24 24"><path d="M12 8v5M12 16h.01" /><circle cx="12" cy="12" r="9" /></svg>
        <div class="invite-box">
          <span><b>{labels.ajustes.inviteLink}:</b> <span class="mono invite-url">{inviteLink}</span></span>
          <div class="invite-actions">
            <button class="btn btn-primary btn-sm" onclick={() => copyText(inviteLink, labels.ajustes.inviteLinkCopied)}>{labels.ajustes.copyInviteLink}</button>
            {#if inviteCode}
              <button class="btn btn-kraft btn-sm" onclick={() => copyText(inviteCode, labels.ajustes.inviteCodeCopied)}>{labels.ajustes.copyInviteCode}</button>
            {/if}
          </div>
        </div>
      </div>
    {/if}
    <div class="panel">
      <table class="table">
        <thead>
          <tr>
            <th>{labels.ajustes.inviteLabel}</th>
            <th>{labels.ajustes.createdAt}</th>
            <th>{labels.ajustes.expiresAt}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each invites as invite (invite.id)}
            <tr>
              <td data-label={labels.ajustes.inviteLabel}>{invite.label || '—'}</td>
              <td data-label={labels.ajustes.createdAt} class="num">{formatDate(invite.createdAt)}</td>
              <td data-label={labels.ajustes.expiresAt} class="num">{formatDate(invite.expiresAt)}</td>
              <td class="acc"><button class="btn btn-danger btn-sm" onclick={() => revokeInvite(invite.id)}>{labels.ajustes.revoke}</button></td>
            </tr>
          {/each}
          {#if invites.length === 0}
            <tr><td colspan="4" class="muted">{labels.ajustes.inviteEmpty}</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
    <div class="add-row">
      <input class="input" bind:value={inviteLabel} placeholder={labels.ajustes.inviteLabel} />
      <button class="btn btn-primary" onclick={createInvite} disabled={busy}>{labels.ajustes.inviteCreate}</button>
    </div>
  </div>

  <div>
    <h3 class="block-title">{labels.ajustes.sessions}</h3>
    <div class="panel">
      <table class="table">
        <thead>
          <tr>
            <th>{labels.roleSpectator}</th>
            <th>{labels.ajustes.lastSeen}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each sessions as session (session.id)}
            <tr>
              <td data-label={labels.roleSpectator}>{session.label || labels.roleSpectator}</td>
              <td data-label={labels.ajustes.lastSeen} class="num">{formatDate(session.lastSeen)}</td>
              <td class="acc"><button class="btn btn-danger btn-sm" onclick={() => revokeSession(session.id)}>{labels.ajustes.revoke}</button></td>
            </tr>
          {/each}
          {#if sessions.length === 0}
            <tr><td colspan="3" class="muted">{labels.ajustes.sessionsEmpty}</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  </div>
</div>
