<script lang="ts">
  import { api } from '../lib/api';
  import { labels } from '../lib/labels';
  import type { PanelState, Session, TunnelInfo } from '../lib/types';
  import PushToggle from './PushToggle.svelte';

  interface Props {
    session: Session
    tunnel: TunnelInfo | null
    revision?: number
    notify?: (message: string, error?: boolean) => void
    onNavigate?: (screen: string) => void
  }
  let { session, tunnel, revision = 0, notify, onNavigate }: Props = $props()

  let panelState = $state<PanelState | null>(null)
  let busy = $state(false)

  async function load () {
    try {
      panelState = await api<PanelState>('/api/panel/state')
    } catch {
      panelState = null
    }
  }

  $effect(() => {
    void revision
    void load()
  })

  async function publish () {
    busy = true
    try {
      await api('/api/panel/list/publish', { method: 'POST' })
      notify?.(labels.resumen.published)
      await load()
    } catch (error) {
      notify?.((error as Error).message, true)
    } finally {
      busy = false
    }
  }

  async function sendTest () {
    busy = true
    try {
      await api('/api/panel/targets/test', { method: 'POST' })
      notify?.(labels.resumen.testSent)
    } catch (error) {
      notify?.((error as Error).message, true)
    } finally {
      busy = false
    }
  }

  const statusText = $derived(tunnel ? (labels.tunnelStatus[tunnel.status] ?? tunnel.status) : '—')
  const statusClass = $derived(tunnel?.status === 'running' ? 'herb' : tunnel?.status === 'errored' ? 'jam' : 'honey')
  const wa = $derived(panelState?.snapshot.whatsapp ?? null)
  const botStatus = $derived(wa && wa.status ? (labels.whatsappStatus[wa.status] ?? wa.status) : '—')
  const botClass = $derived(wa?.status === 'connected' ? 'herb' : wa?.status === 'disabled' || !wa?.status ? 'neutral' : 'honey')

  const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'America/Bogota' }).format(new Date())
  const todayList = $derived((panelState?.snapshot.days ?? []).find((day) => day.date === today) ?? null)
  const nextSend = $derived(panelState?.nextSend ?? null)

  const activity = $derived(panelState?.snapshot.activity ?? [])
  const recent = $derived.by(() => {
    const days = panelState?.snapshot.days ?? []
    return [...days].sort((a, b) => b.date.localeCompare(a.date)).slice(0, 5)
  })

  const dateFormat = new Intl.DateTimeFormat('es-CO', { day: 'numeric', month: 'long' })
  function friendlyDate (iso: string) {
    const [year, month, day] = iso.split('-').map(Number)
    if (!year || !month || !day) return iso
    return dateFormat.format(new Date(Date.UTC(year, month - 1, day)))
  }

  function relativeTime (iso: string) {
    const then = new Date(iso).getTime()
    if (Number.isNaN(then)) return ''
    const seconds = Math.max(0, Math.round((Date.now() - then) / 1000))
    if (seconds < 60) return labels.resumen.agoNow
    const minutes = Math.round(seconds / 60)
    if (minutes < 60) return labels.resumen.agoMinutes.replace('{n}', String(minutes))
    const hours = Math.round(minutes / 60)
    if (hours < 24) return labels.resumen.agoHours.replace('{n}', String(hours))
    return labels.resumen.agoDays.replace('{n}', String(Math.round(hours / 24)))
  }

  function kindColor (kind: string) {
    if (kind === 'new') return 'var(--accent)'
    if (kind === 'update') return 'var(--warning)'
    if (kind === 'removed') return 'var(--danger)'
    return 'var(--text-muted)'
  }
</script>

<section class="screen">
  <div class="ledger">
    <div class="tile">
      <div class="label">{labels.resumen.bots}</div>
      <div class="value">{botStatus}</div>
      <div class="foot">{wa?.phone || (wa?.linked ? labels.bots.noNumber : '—')}</div>
    </div>
    <div class="tile">
      <div class="label">{labels.resumen.tunnel}</div>
      <div class="value">{statusText}</div>
      <div class="foot mono">{tunnel?.hostname ?? '—'}</div>
    </div>
    <div class="tile">
      <div class="label">{labels.resumen.unitsToday}</div>
      <div class="value">{todayList ? todayList.list.total : '—'}</div>
      <div class="foot">{todayList ? `${todayList.orders.length + (todayList.orders.length > 1 ? ' pedidos' : ' pedido') }` : labels.resumen.noOrders}</div>
    </div>
    <div class="tile">
      <div class="label">{labels.resumen.nextSend}</div>
      <div class="value">{nextSend ? nextSend.time : '—'}</div>
      <div class="foot">{nextSend ? labels.resumen.targets.replace('{n}', String(nextSend.targets)) : labels.resumen.noNextSend}</div>
    </div>
  </div>

  <div class="cols">
    <div>
      <h3 class="section-label">{labels.resumen.activity}</h3>
      <hr class="section-rule" />
      {#if activity.length > 0}
        <div class="activity">
          {#each activity as event, index (index)}
            <div class="act">
              <span class="dot" style="background:{kindColor(event.kind)}"></span>
              <span class="txt"><b>{event.text}</b></span>
              <span class="when">{relativeTime(event.time)}</span>
            </div>
          {/each}
        </div>
      {:else if recent.length > 0}
        <div class="activity">
          {#each recent as day (day.date)}
            <div class="act">
              <span class="dot" style="background:var(--accent)"></span>
              <span class="txt">
                <b>{labels.resumen.listFor}</b>
                <span>: {friendlyDate(day.date)} · {day.list.total} unidades</span>
              </span>
              <span class="when">{day.orders.length} pedidos</span>
            </div>
          {/each}
        </div>
      {:else}
        <p class="muted">{labels.resumen.noActivity}</p>
      {/if}
    </div>

    <div>
      <div class="recipe">
        <div class="recipe-head">
          <h3>{labels.resumen.access}</h3>
          <span class="pin">{session.local ? labels.resumen.local : labels.resumen.remote}</span>
        </div>
        <div class="bot-line">
          <div class="grow">
            <div class="bname">{session.role === 'host' ? labels.roleHost : labels.roleSpectator}</div>
            <div class="bphone">{session.label || '—'}</div>
          </div>
          <span class="stamp {statusClass}">{session.local ? 'Local' : 'Remoto'}</span>
        </div>
        <div class="bot-line">
          <div class="grow">
            <div class="bname">WhatsApp</div>
            <div class="bphone">{wa?.phone || (wa?.linked ? labels.bots.noNumber : '—')}</div>
          </div>
          <span class="stamp {botClass}">{botStatus}</span>
        </div>
      </div>

      <div class="recipe">
        <div class="recipe-head">
          <h3>{labels.push.title}</h3>
        </div>
        <PushToggle />
      </div>

      {#if session.role === 'host'}
        <h3 class="section-label">{labels.resumen.quick}</h3>
        <hr class="section-rule" />
        <div class="quick">
          <button class="btn btn-primary" onclick={() => onNavigate?.('bots')}>
            <svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14" /></svg>
            {labels.resumen.addAccount}
          </button>
          <button class="btn btn-kraft" onclick={publish} disabled={busy}>
            <svg viewBox="0 0 24 24"><path d="M4 12h16M4 12l5-5M4 12l5 5" /><path d="M14 6h6v12h-6" /></svg>
            {labels.resumen.publishToday}
          </button>
          <button class="btn btn-kraft" onclick={sendTest} disabled={busy}>
            <svg viewBox="0 0 24 24"><path d="M4 5h16v11H9l-5 4z" /><path d="M8.5 9.5h7" /></svg>
            {labels.resumen.sendTest}
          </button>
        </div>
      {/if}
    </div>
  </div>
</section>
