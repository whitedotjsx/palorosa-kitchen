<script lang="ts">
  import { labels } from '../lib/labels'
  import { api } from '../lib/api'
  import type { Bot, Target } from '../lib/types'

  interface Props {
    revision?: number
    notify?: (message: string, error?: boolean) => void
  }
  let { revision = 0, notify }: Props = $props()

  let targets = $state<Target[]>([])
  let bots = $state<Bot[]>([])
  let editing = $state<Target>(blank())
  let newTime = $state('')
  let quietFrom = $state('')
  let quietTo = $state('')
  let busy = $state(false)

  function blank (): Target {
    return {
      id: '',
      label: '',
      phone: '',
      kinds: ['new', 'update'],
      schedule: { mode: 'immediate', times: [], timezone: 'America/Bogota', quietHours: [] },
      enabled: true,
    }
  }

  async function load () {
    try {
      targets = (await api<{ targets: Target[] }>('/api/panel/targets')).targets
      bots = (await api<{ bots: Bot[] }>('/api/panel/bots')).bots
    } catch {
      targets = []
    }
  }

  $effect(() => {
    void revision
    void load()
  })

  function edit (target: Target) {
    const copy = $state.snapshot(target)
    editing = copy
    quietFrom = copy.schedule.quietHours?.[0] ?? ''
    quietTo = copy.schedule.quietHours?.[1] ?? ''
  }

  function addNew () {
    editing = blank()
    quietFrom = ''
    quietTo = ''
  }

  function toggleKind (kind: string) {
    const index = editing.kinds.indexOf(kind)
    if (index >= 0) editing.kinds.splice(index, 1)
    else editing.kinds.push(kind)
  }

  function addTime () {
    const value = newTime.trim()
    if (/^\d{1,2}:\d{2}$/.test(value) && !editing.schedule.times.includes(value)) editing.schedule.times.push(value)
    newTime = ''
  }

  function removeTime (time: string) {
    editing.schedule.times = editing.schedule.times.filter((item) => item !== time)
  }

  function setMode (mode: 'immediate' | 'times') {
    editing.schedule.mode = mode
  }

  async function save () {
    const phone = editing.phone.replace(/[^0-9]/g, '')
    if (!phone) {
      notify?.('Falta el número', true)
      return
    }
    editing.phone = phone
    editing.schedule.quietHours = quietFrom && quietTo ? [quietFrom, quietTo] : []
    busy = true
    try {
      const body = JSON.stringify(editing)
      if (editing.id) {
        await api(`/api/panel/targets/${editing.id}`, { method: 'PATCH', body })
      } else {
        const result = await api<{ target: Target }>('/api/panel/targets', { method: 'POST', body })
        editing = structuredClone(result.target)
      }
      await load()
      notify?.(labels.destinos.saved)
    } catch (error) {
      notify?.((error as Error).message, true)
    } finally {
      busy = false
    }
  }

  async function remove () {
    if (!editing.id || !window.confirm(labels.destinos.removeConfirm)) return
    try {
      await api(`/api/panel/targets/${editing.id}`, { method: 'DELETE' })
      addNew()
      await load()
    } catch (error) {
      notify?.((error as Error).message, true)
    }
  }

  async function test () {
    if (!editing.id) return
    try {
      await api(`/api/panel/targets/${editing.id}/test`, { method: 'POST' })
      notify?.(labels.destinos.testSent)
    } catch (error) {
      notify?.((error as Error).message, true)
    }
  }

  function scheduleText (target: Target) {
    return target.schedule.mode === 'times' && target.schedule.times.length
      ? target.schedule.times.join(' · ')
      : labels.destinos.immediate
  }
</script>

<section class="screen">
  <div class="note" style="margin-bottom:20px">
    <svg viewBox="0 0 24 24"><path d="M12 8v5M12 16h.01" /><circle cx="12" cy="12" r="9" /></svg>
    <span>{labels.destinos.note}</span>
  </div>

  <div class="with-drawer">
    <div class="panel">
      <table class="table">
        <thead>
          <tr>
            <th>{labels.destinos.target}</th>
            <th>{labels.destinos.events}</th>
            <th>{labels.destinos.times}</th>
            <th>{labels.destinos.enabled}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each targets as target (target.id)}
            <tr>
              <td class="cell-acct" data-label={labels.destinos.target}>
                <b>{target.label || target.phone}</b>
                <span>{target.label ? target.phone : ''}</span>
              </td>
              <td data-label={labels.destinos.events}>
                <div class="chips">
                  {#if target.kinds.includes('new')}<span class="chip">{labels.destinos.kindsNew}</span>{/if}
                  {#if target.kinds.includes('update')}<span class="chip">{labels.destinos.kindsUpdated}</span>{/if}
                  {#if target.kinds.includes('today')}<span class="chip">{labels.destinos.kindToday}</span>{/if}
                  {#if target.kinds.includes('tomorrow')}<span class="chip">{labels.destinos.kindTomorrow}</span>{/if}
                </div>
              </td>
              <td data-label={labels.destinos.times} class="num">{scheduleText(target)}</td>
              <td data-label={labels.destinos.enabled}>
                <span class="stamp" class:herb={target.enabled} class:neutral={!target.enabled}>
                  {target.enabled ? labels.destinos.enabled : labels.destinos.disabled}
                </span>
              </td>
              <td class="acc"><button class="btn btn-kraft btn-sm" onclick={() => edit(target)}>✎</button></td>
            </tr>
          {/each}
          {#if targets.length === 0}
            <tr><td colspan="5" class="muted">{labels.destinos.empty}</td></tr>
          {/if}
        </tbody>
      </table>
    </div>

    <aside class="recipe editor-side" style="margin:0">
      <div class="recipe-head">
        <h3>{editing.id ? labels.destinos.editTarget : labels.destinos.newTarget}</h3>
      </div>

      <div class="field">
        <label class="label" for="d-etq">{labels.destinos.label}</label>
        <input class="input" id="d-etq" bind:value={editing.label} />
      </div>
      <div class="field">
        <label class="label" for="d-num">{labels.destinos.phone}</label>
        <input class="input" id="d-num" bind:value={editing.phone} placeholder="+57 3…" />
      </div>

      <div class="field">
        <label class="label" for="d-bot">{labels.destinos.bot}</label>
        <div class="select">
          <select id="d-bot" bind:value={editing.botId}>
            <option value="">{labels.destinos.botAuto}</option>
            {#each bots as bot (bot.id)}<option value={bot.id}>{bot.name}</option>{/each}
          </select>
        </div>
      </div>

      <div class="field">
        <span class="label">{labels.destinos.events}</span>
        <div class="checks">
          <label class="check"><input type="checkbox" checked={editing.kinds.includes('new')} onchange={() => toggleKind('new')} /> {labels.destinos.kindsNew}</label>
          <label class="check"><input type="checkbox" checked={editing.kinds.includes('update')} onchange={() => toggleKind('update')} /> {labels.destinos.kindsUpdated}</label>
          <label class="check"><input type="checkbox" checked={editing.kinds.includes('today')} onchange={() => toggleKind('today')} /> {labels.destinos.kindToday}</label>
          <label class="check"><input type="checkbox" checked={editing.kinds.includes('tomorrow')} onchange={() => toggleKind('tomorrow')} /> {labels.destinos.kindTomorrow}</label>
        </div>
      </div>

      <div class="field">
        <span class="label">{labels.destinos.times}</span>
        <div class="seg">
          <button type="button" class:active={editing.schedule.mode === 'immediate'} onclick={() => setMode('immediate')}>{labels.destinos.modeImmediate}</button>
          <button type="button" class:active={editing.schedule.mode === 'times'} onclick={() => setMode('times')}>{labels.destinos.modeTimes}</button>
        </div>
      </div>

      {#if editing.schedule.mode === 'times'}
        <div class="field">
          <div class="chips">
            {#each editing.schedule.times as time (time)}
              <span class="chip">{time} <button type="button" class="x" aria-label="Quitar hora" onclick={() => removeTime(time)}>✕</button></span>
            {/each}
          </div>
          <div class="add-row">
            <input class="input" bind:value={newTime} placeholder="06:30" onkeydown={(event) => event.key === 'Enter' && addTime()} />
            <button class="btn btn-kraft" onclick={addTime}>{labels.destinos.addTime}</button>
          </div>
        </div>
        <div class="field">
          <span class="label">{labels.destinos.quiet}</span>
          <div class="field-row">
            <input class="input" bind:value={quietFrom} placeholder="21:00" aria-label={labels.destinos.quietFrom} />
            <input class="input" bind:value={quietTo} placeholder="06:00" aria-label={labels.destinos.quietTo} />
          </div>
        </div>
      {/if}

      <div class="field">
        <label class="check"><input type="checkbox" bind:checked={editing.enabled} /> {labels.destinos.enabled}</label>
      </div>

      <div class="drawer-foot">
        {#if editing.id}
          <button class="btn btn-ghost btn-sm" onclick={test}>{labels.destinos.test}</button>
          <button class="btn btn-danger btn-sm" onclick={remove}>{labels.destinos.remove}</button>
        {/if}
        <button class="btn btn-primary btn-sm" onclick={save} disabled={busy}>{labels.destinos.save}</button>
        <button class="btn btn-kraft btn-sm" onclick={addNew}>{labels.destinos.add}</button>
      </div>
    </aside>
  </div>
</section>
