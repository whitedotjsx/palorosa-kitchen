<script lang="ts">
  import { api } from '../lib/api'
  import { labels } from '../lib/labels'
  import logo from '../assets/palorosa-logo.png'

  interface Props {
    onDone: () => void
  }
  let { onDone }: Props = $props()

  let value = $state('')
  let busy = $state(false)
  let error = $state('')

  // Accept either the bare token or the whole invite link (…/?invite=TOKEN).
  function extractToken (raw: string): string {
    const trimmed = raw.trim()
    try {
      const fromUrl = new URL(trimmed).searchParams.get('invite')
      if (fromUrl) return fromUrl
    } catch {
      // Not a URL: treat it as the token itself.
    }
    return trimmed
  }

  async function submit (event: SubmitEvent) {
    event.preventDefault()
    const token = extractToken(value)
    if (!token) return
    busy = true
    error = ''
    try {
      await api('/api/panel/session', { method: 'POST', body: JSON.stringify({ token }) })
      onDone()
    } catch {
      error = labels.invite.error
    } finally {
      busy = false
    }
  }
</script>

<main class="gate">
  <section class="card">
    <img class="logo" src={logo} alt={labels.brand} />
    <p class="kicker">{labels.appTitle}</p>
    <h1>{labels.invite.title}</h1>
    <p class="intro">{labels.invite.intro}</p>

    <form onsubmit={submit} novalidate>
      <label class="label" for="invite-token">{labels.invite.token}</label>
      <div class="input-wrap" class:has-error={!!error}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="8" cy="15" r="4" /><path d="M11 12l9-9M17 6l3 3M14 9l2 2" /></svg>
        <input
          id="invite-token"
          bind:value
          oninput={() => (error = '')}
          autocomplete="off"
          autocapitalize="off"
          spellcheck="false"
          placeholder={labels.invite.tokenHint}
          aria-invalid={!!error}
          aria-describedby={error ? 'invite-error' : 'invite-help'}
        />
      </div>
      {#if error}
        <p class="error" id="invite-error" role="alert">{error}</p>
      {/if}
      <button class="btn btn-primary submit" type="submit" disabled={busy || !value.trim()}>
        {busy ? labels.invite.checking : labels.invite.submit}
      </button>
    </form>

    <p class="help" id="invite-help">{labels.invite.help}</p>
  </section>
</main>

<style>
  .gate {
    flex: 1;
    width: 100%;
    box-sizing: border-box;
    min-height: 100dvh;
    display: grid;
    place-items: center;
    padding: 24px 16px;
    background:
      radial-gradient(circle at 15% 0%, var(--blush) 0, transparent 45%),
      radial-gradient(circle at 100% 100%, var(--accent-soft) 0, transparent 40%),
      var(--bg);
  }

  .card {
    width: min(400px, 100%);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 20px;
    padding: 32px 28px 24px;
    box-shadow: 0 24px 60px -28px rgba(106, 75, 32, 0.35);
    text-align: center;
  }

  .logo {
    width: 168px;
    max-width: 70%;
    height: auto;
    margin: 0 auto 6px;
    display: block;
  }

  .kicker {
    margin: 0 0 20px;
    font-size: 11px;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  h1 {
    margin: 0 0 6px;
    font-family: var(--font-display);
    font-size: 21px;
    font-weight: 600;
    color: var(--text);
  }

  .intro {
    margin: 0 0 24px;
    font-size: 13.5px;
    line-height: 1.5;
    color: var(--text-muted);
  }

  form {
    text-align: left;
  }

  .label {
    display: block;
    margin-bottom: 6px;
    font-size: 12.5px;
    font-weight: 500;
    color: var(--label);
  }

  .input-wrap {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 14px;
    background: #fff;
    border: 1px solid var(--border-strong);
    border-radius: 12px;
    transition: border-color 0.15s, box-shadow 0.15s;
  }

  .input-wrap:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }

  .input-wrap.has-error {
    border-color: var(--danger);
  }

  .input-wrap svg {
    width: 18px;
    height: 18px;
    flex: none;
    fill: none;
    stroke: var(--brown-medium);
    stroke-width: 1.8;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .input-wrap input {
    flex: 1;
    min-width: 0;
    height: 48px;
    border: 0;
    outline: none;
    background: transparent;
    font: inherit;
    font-size: 15px;
    color: var(--text);
  }

  .input-wrap input::placeholder {
    color: var(--olive);
  }

  .error {
    margin: 8px 0 0;
    padding: 8px 12px;
    border-radius: 10px;
    background: var(--warning-soft);
    color: var(--danger);
    font-size: 12.5px;
  }

  .submit {
    width: 100%;
    justify-content: center;
    height: 48px;
    margin-top: 16px;
    border-radius: 12px;
    font-size: 15px;
  }

  .help {
    margin: 20px 0 0;
    padding-top: 16px;
    border-top: 1px dashed var(--border);
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-muted);
  }

  @media (max-width: 480px) {
    .gate {
      place-items: start center;
      padding-top: 12vh;
    }

    .card {
      padding: 28px 20px 20px;
    }
  }
</style>
