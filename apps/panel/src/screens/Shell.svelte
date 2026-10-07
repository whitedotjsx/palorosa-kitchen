<script lang="ts">
  import { onMount } from 'svelte'
  import type { Session, TunnelInfo } from '../lib/types'
  import { api } from '../lib/api'
  import { labels } from '../lib/labels'
  import logo from '../assets/palorosa-logo.png'
  import Resumen from './Resumen.svelte'
  import Ajustes from './Ajustes.svelte'
  import Bots from './Bots.svelte'
  import Destinos from './Destinos.svelte'
  import Pedidos from './Pedidos.svelte'
  import Lista from './Lista.svelte'
  import Catalogo from './Catalogo.svelte'

  interface Props {
    session: Session
    onLogout: () => void
  }
  let { session, onLogout }: Props = $props()

  type Screen = 'resumen' | 'bots' | 'destinos' | 'pedidos' | 'lista' | 'catalogo' | 'ajustes'

  interface NavItem {
    id: Screen
    label: string
    title: string
    sub: string
    icon: string
  }

  const items: NavItem[] = [
    {
      id: 'resumen',
      label: 'Resumen',
      title: 'Resumen',
      sub: 'La cocina de hoy, de un vistazo',
      icon: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
    },
    {
      id: 'bots',
      label: 'Cuentas',
      title: 'Cuentas de WhatsApp',
      sub: 'Las cuentas que atienden la cocina',
      icon: '<path d="M4 5h16v11H9l-5 4z"/><path d="M8.5 9.5h7M8.5 12.5h4"/>',
    },
    {
      id: 'destinos',
      label: 'Destinos',
      title: 'Destinos y horarios',
      sub: 'A quién avisamos, con qué bot y a qué hora',
      icon: '<path d="M21 3 3 10l7 3 3 7z"/><path d="M21 3 10 13"/>',
    },
    {
      id: 'pedidos',
      label: 'Pedidos',
      title: 'Pedidos',
      sub: 'Lo que entró desde la tienda',
      icon: '<path d="M6 3h12v18l-3-2-3 2-3-2-3 2z"/><path d="M9 8h6M9 12h6"/>',
    },
    {
      id: 'lista',
      label: 'Lista',
      title: 'Lista de cocina',
      sub: 'Lo que hay que preparar hoy',
      icon: '<path d="M4 6h.01M4 12h.01M4 18h.01M8.5 6h11M8.5 12h11M8.5 18h11"/>',
    },
    {
      id: 'catalogo',
      label: 'Catálogo',
      title: 'Catálogo y hoja de cocina',
      sub: 'Productos, recetas y el formato del PDF',
      icon: '<path d="M4 4.5A1.5 1.5 0 0 1 5.5 3H20v15H5.5A1.5 1.5 0 0 0 4 19.5z"/><path d="M4 19.5A1.5 1.5 0 0 0 5.5 21H20v-3"/><path d="M8 7h8M8 11h6"/>',
    },
    {
      id: 'ajustes',
      label: 'Ajustes',
      title: 'Ajustes',
      sub: 'Túnel, arranque y diagnóstico',
      icon: '<circle cx="12" cy="12" r="3.2"/><path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.1 5.1l2.1 2.1M16.8 16.8l2.1 2.1M18.9 5.1l-2.1 2.1M7.2 16.8l-2.1 2.1"/>',
    },
  ]

  const spectatorScreens: Screen[] = ['resumen', 'pedidos', 'lista']
  const allowed = $derived(items.filter((item) => session.role === 'host' || spectatorScreens.includes(item.id)))

  let screen = $state<Screen>('resumen')
  const current = $derived(allowed.find((item) => item.id === screen) ?? allowed[0])

  let sidebarOpen = $state(false)
  let toast = $state('')
  let toastError = $state(false)
  let tunnel = $state<TunnelInfo | null>(null)
  let revision = $state(0)

  function notify (message: string, error = false) {
    toast = message
    toastError = error
    window.setTimeout(() => {
      toast = ''
    }, 2400)
  }

  async function loadTunnel () {
    try {
      tunnel = await api<TunnelInfo>('/api/panel/tunnel')
    } catch {
      tunnel = null
    }
  }

  async function logout () {
    try {
      await api('/api/panel/session', { method: 'DELETE' })
    } catch {
      // The refresh below resets the view anyway.
    }
    onLogout()
  }

  onMount(() => {
    void loadTunnel()
    // Live updates from the server (Phase 4): the screens refetch on any event.
    const source = new EventSource('/api/panel/events')
    source.addEventListener('orders', () => (revision += 1))
    source.addEventListener('whatsapp', () => (revision += 1))
    source.addEventListener('settings', () => {
      revision += 1
      void loadTunnel()
    })
    source.addEventListener('tunnel', () => void loadTunnel())
    return () => source.close()
  })

  const tunnelText = $derived(tunnel ? (labels.tunnelStatus[tunnel.status] ?? tunnel.status) : '…')
  const chip = $derived(tunnel?.hostname || (session.local ? labels.brand : labels.roleSpectator))
</script>

<svelte:window onkeydown={(event) => event.key === 'Escape' && (sidebarOpen = false)} />

<aside class="sidebar" class:open={sidebarOpen}>
  <div class="brand">
    <img class="brand-logo" src={logo} alt={`${labels.brand} ${labels.appTitle}`} />
  </div>

  <nav class="nav" aria-label="Secciones">
    {#each allowed as item (item.id)}
      <button data-screen={item.id} class:active={screen === item.id} onclick={() => { screen = item.id; sidebarOpen = false }}>
        <svg viewBox="0 0 24 24">{@html item.icon}</svg>
        {item.label}
      </button>
    {/each}
  </nav>

  <div class="side-foot">
    <div class="row">
      <span class="dot" class:herb={tunnel?.status === 'running'} class:honey={tunnel?.status !== 'running'}></span>
      Túnel {tunnelText}
    </div>
    <div class="row">
      <span class="dot herb"></span>
      {session.role === 'host' ? labels.roleHost : labels.roleSpectator}{session.label ? ` · ${session.label}` : ''}
    </div>
    <button class="btn btn-kraft btn-sm" onclick={logout}>{labels.logout}</button>
  </div>
</aside>

<button type="button" class="scrim" class:on={sidebarOpen} aria-label="Cerrar menú" onclick={() => (sidebarOpen = false)}></button>

<div class="main">
  <header class="topbar">
    <button class="menu-btn" type="button" aria-label="Abrir menú" aria-expanded={sidebarOpen} onclick={() => (sidebarOpen = !sidebarOpen)}>
      <svg viewBox="0 0 24 24"><path d="M4 7h16M4 12h16M4 17h16"/></svg>
    </button>
    <div class="titles">
      <h1>{current?.title ?? ''}</h1>
      <span class="sub">{current?.sub ?? ''}</span>
    </div>
    <div class="right">
      <span class="stamp" class:herb={session.role === 'host'} class:honey={session.role !== 'host'}>
        {session.role === 'host' ? labels.roleHost : labels.roleSpectator}
      </span>
      <span class="kraft-chip">{chip}</span>
      <div class="avatar">{labels.brand.slice(0, 1)}</div>
    </div>
  </header>

  <main class="content">
    {#if current?.id === 'resumen'}
      <Resumen {session} {tunnel} {revision} {notify} onNavigate={(next) => { if (allowed.some((item) => item.id === next)) screen = next as Screen; sidebarOpen = false }} />
    {:else if current?.id === 'ajustes'}
      <Ajustes {notify} {tunnel} reloadTunnel={loadTunnel} />
    {:else if current?.id === 'bots'}
      <Bots {revision} {notify} />
    {:else if current?.id === 'destinos'}
      <Destinos {revision} {notify} />
    {:else if current?.id === 'pedidos'}
      <Pedidos {revision} role={session.role} {notify} />
    {:else if current?.id === 'lista'}
      <Lista {revision} {notify} />
    {:else if current?.id === 'catalogo'}
      <Catalogo {notify} />
    {/if}
  </main>
</div>

<div class="toast" role="status" style:opacity={toast ? 1 : 0} style:pointer-events={toast ? 'auto' : 'none'}>
  <svg viewBox="0 0 24 24"><path d={toastError ? 'M6 6l12 12M18 6L6 18' : 'M5 12.5l4.5 4.5L19 7'} /></svg>
  {toast}
</div>
