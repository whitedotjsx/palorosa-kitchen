<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { labels } from '../lib/labels'

  // The desktop shell raises native Windows toasts from Go: WebView2 cannot
  // deliver Web Push and its host denies the web Notification permission.
  const desktop = (window as unknown as { __palorosaDesktop?: boolean }).__palorosaDesktop === true

  let supported = $state(false)
  let enabled = $state(false)
  let busy = $state(false)
  let error = $state('')

  onMount(async () => {
    if (desktop) return
    supported = 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
    if (!supported) return
    try {
      const registration = await navigator.serviceWorker.ready
      enabled = (await registration.pushManager.getSubscription()) !== null
    } catch {
      // Registration is not ready yet; the button below still works.
    }
  })

  function keyToBytes (base64: string) {
    const padding = '='.repeat((4 - (base64.length % 4)) % 4)
    const raw = atob((base64 + padding).replace(/-/g, '+').replace(/_/g, '/'))
    const bytes = new Uint8Array(raw.length)
    for (let index = 0; index < raw.length; index += 1) bytes[index] = raw.charCodeAt(index)
    return bytes
  }

  function friendlyError (err: unknown) {
    const message = (err as Error)?.message ?? ''
    if (/permission denied|NotAllowedError/i.test(message)) {
      return Notification.permission === 'denied' ? labels.push.blocked : labels.push.denied
    }
    return message || labels.push.failed
  }

  async function enable () {
    busy = true
    error = ''
    try {
      if (Notification.permission === 'denied') {
        error = labels.push.blocked
        return
      }
      const permission = await Notification.requestPermission()
      if (permission !== 'granted') {
        error = labels.push.denied
        return
      }
      const info = await api<{ enabled: boolean; publicKey: string }>('/api/panel/push')
      if (!info.enabled || !info.publicKey) {
        error = labels.push.unavailable
        return
      }
      const registration = await navigator.serviceWorker.ready
      const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: keyToBytes(info.publicKey),
      })
      await api('/api/panel/push', { method: 'POST', body: JSON.stringify(subscription.toJSON()) })
      enabled = true
    } catch (err) {
      error = friendlyError(err)
    } finally {
      busy = false
    }
  }

  async function disable () {
    busy = true
    error = ''
    try {
      const registration = await navigator.serviceWorker.ready
      const subscription = await registration.pushManager.getSubscription()
      if (subscription) {
        await api('/api/panel/push', { method: 'DELETE', body: JSON.stringify({ endpoint: subscription.endpoint }) })
        await subscription.unsubscribe()
      }
      enabled = false
    } catch (err) {
      error = friendlyError(err)
    } finally {
      busy = false
    }
  }
</script>

{#if desktop}
  <div class="push-line">
    <div class="grow">
      <div class="bname">{labels.push.desktopOn}</div>
      <div class="push-hint">{labels.push.desktopHint}</div>
    </div>
  </div>
{:else if !supported}
  <p class="helper">{labels.push.unsupported}</p>
{:else}
  <div class="push-line">
    <div class="grow">
      <div class="bname">{enabled ? labels.push.enabled : labels.push.off}</div>
      <div class="push-hint">{labels.push.hint}</div>
    </div>
    <button
      class="btn {enabled ? 'btn-kraft' : 'btn-primary'} btn-sm"
      onclick={enabled ? disable : enable}
      disabled={busy}
    >
      {busy ? labels.push.working : enabled ? labels.push.disable : labels.push.enable}
    </button>
  </div>
  {#if error}<p class="helper push-error">{error}</p>{/if}
{/if}

<style>
  .push-line { display: flex; align-items: center; gap: 12px; }
  .push-line .grow { flex: 1 1 auto; min-width: 0; }
  .push-line .bname { font-family: var(--font-display); font-weight: 600; font-size: 15px; }
  .push-hint { color: var(--text-muted); font-size: 12px; margin-top: 2px; }
  .push-error { color: var(--danger); }
</style>
