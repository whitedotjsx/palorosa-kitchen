<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from './lib/api'
  import { labels } from './lib/labels'
  import type { Session } from './lib/types'
  import Invite from './screens/Invite.svelte'
  import Shell from './screens/Shell.svelte'

  let session = $state<Session | null>(null)
  let loading = $state(true)

  async function refresh () {
    loading = true
    try {
      session = await api<Session>('/api/panel/session')
    } catch {
      session = { authenticated: false, role: '', local: false, label: '' }
    } finally {
      loading = false
    }
  }

  onMount(async () => {
    const url = new URL(window.location.href)
    const invite = url.searchParams.get('invite')
    if (invite) {
      url.searchParams.delete('invite')
      window.history.replaceState({}, '', url.pathname + url.search)
      try {
        await api('/api/panel/session', { method: 'POST', body: JSON.stringify({ token: invite }) })
      } catch {
        // The Invite screen below lets the user retry by hand.
      }
    }
    await refresh()
  })
</script>

{#if loading}
  <div class="boot">{labels.loading}</div>
{:else if !session?.authenticated}
  <Invite onDone={refresh} />
{:else}
  <Shell {session} onLogout={refresh} />
{/if}

<style>
  .boot {
    display: grid;
    place-items: center;
    min-height: 100vh;
    color: var(--text-muted);
  }
</style>
