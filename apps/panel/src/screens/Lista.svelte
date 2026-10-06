<script lang="ts">
  import { labels } from '../lib/labels'
  import { api } from '../lib/api'
  import type { PanelEntry, PanelEntrySource, PanelList } from '../lib/types'
  import { loadTemplate, printSheet, sheetHtml } from '../lib/print'
  import { day, longDate, rangeDates, rangeLabel, shortDate } from '../lib/day.svelte'
  import { mergeDayLists } from '../lib/multi-day'
  import DayPicker from '../components/DayPicker.svelte'
  import SyncButton from '../components/SyncButton.svelte'

  interface Props {
    revision?: number
    notify?: (message: string, error?: boolean) => void
  }
  let { revision = 0, notify }: Props = $props()

  let list = $state<PanelList | null>(null)

  let printing = $state(false)

  // Per-unit provenance: the breakfast/add-on lines each count comes from.
  // Only the first few show until the row is expanded.
  const sourcePreview = 3
  let expandedSources = $state<string[]>([])

  function toggleSources (unitId: string) {
    expandedSources = expandedSources.includes(unitId)
      ? expandedSources.filter((item) => item !== unitId)
      : [...expandedSources, unitId]
  }

  function visibleSources (entry: PanelEntry): PanelEntrySource[] {
    const sources = entry.sources ?? []
    return expandedSources.includes(entry.unitId) ? sources : sources.slice(0, sourcePreview)
  }

  function sourceKey (source: PanelEntrySource) {
    return `${source.date ?? ''}|${source.productText}|${source.orderNumber ?? ''}|${source.source ?? ''}`
  }

  function sourceToggleText (entry: PanelEntry) {
    const hidden = (entry.sources?.length ?? 0) - sourcePreview
    return labels.lista.originMore.replace('{n}', String(hidden))
  }

  // Prints the kitchen sheet with the saved layout (Catálogo → Formato PDF).
  // "Guardar como PDF" in the print dialog produces the PDF.
  async function printList () {
    if (!list || printing) return
    printing = true
    try {
      const { template } = await loadTemplate()
      printSheet(sheetHtml(list, template, day.range ? rangeText : longDate(day.date)))
    } catch (error) {
      notify?.((error as Error).message, true)
    } finally {
      printing = false
    }
  }

  function measureText (entry: { measure: string }) {
    if (!entry.measure) return ''
    const measure = labels.measures[entry.measure] ?? entry.measure
    return measure === '-' ? '' : measure
  }

  function reasonText (reason: string) {
    return labels.lista.unresolvedReasons[reason] ?? reason
  }

  const unresolvedTitle = $derived(
    (list?.unresolved ?? 0) === 1
      ? labels.lista.unresolvedOne
      : labels.lista.unresolvedMany.replace('{n}', String(list?.unresolved ?? 0)),
  )

  const rangeText = $derived(day.range ? rangeLabel(day.from, day.to) : '')
  const rangeCount = $derived(day.range ? rangeDates(day.from, day.to).length : 0)

  function buildText () {
    const title = day.range ? labels.lista.rangeTitle.replace('{range}', rangeText) : labels.lista.title
    const lines = [title, '']
    for (const group of groups) {
      lines.push((labels.categories[group.category] ?? group.category).toUpperCase())
      for (const entry of group.entries) {
        const parts = [`${entry.quantity} x ${entry.name}`]
        const measure = measureText(entry)
        if (measure) parts.push(measure)
        if (entry.note) parts.push(entry.note)
        lines.push(parts.join(' · '))
      }
      lines.push('')
    }
    return lines.join('\n').trim()
  }

  async function copyList () {
    try {
      await navigator.clipboard.writeText(buildText())
      notify?.(labels.lista.copied)
    } catch {
      notify?.(labels.lista.copyFailed, true)
    }
  }

  async function load () {
    const mode = day.range
    const date = day.date
    const from = day.from
    const to = day.to
    try {
      if (mode) {
        const dates = rangeDates(from, to)
        const results = await Promise.all(dates.map(async (value) => ({
          date: value,
          list: (await api<{ list: PanelList }>(`/api/panel/list?date=${value}`)).list,
        })))
        if (!day.range || day.from !== from || day.to !== to) return
        list = mergeDayLists(results)
      } else {
        const result = await api<{ list: PanelList }>(`/api/panel/list?date=${date}`)
        if (day.range || date !== day.date) return
        list = result.list
      }
    } catch {
      if (mode) {
        if (day.range && day.from === from && day.to === to) list = null
      } else if (!day.range && date === day.date) {
        list = null
      }
    }
  }

  $effect(() => {
    void revision
    void day.date
    void day.range
    void day.from
    void day.to
    void load()
  })

  const groups = $derived.by(() => {
    const entries = list?.entries ?? []
    if (entries.length === 0) return []
    const map = new Map<string, typeof entries>()
    for (const entry of entries) {
      const key = entry.category || 'other'
      if (!map.has(key)) map.set(key, [])
      map.get(key)!.push(entry)
    }
    return [...map.entries()].map(([category, items]) => ({ category, entries: items }))
  })
</script>

<section class="screen">
  <div class="screen-head">
    <div class="day-bar">
      <DayPicker range />
      <SyncButton
        {revision}
        date={day.range ? day.from : day.date}
        dates={day.range ? rangeDates(day.from, day.to) : undefined}
        {notify}
        onsynced={load}
      />
    </div>
    <div class="actions">
      <button class="btn btn-kraft" onclick={printList} disabled={printing || !list || (list.entries?.length ?? 0) === 0}>
        <svg viewBox="0 0 24 24"><path d="M6 9V4h12v5" /><rect x="4" y="9" width="16" height="8" rx="1.5" /><path d="M7 17v3h10v-3" /></svg>
        {labels.lista.print}
      </button>
      <button class="btn btn-kraft" onclick={copyList} disabled={!list || (list.entries?.length ?? 0) === 0}>
        <svg viewBox="0 0 24 24"><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2" /></svg>
        {labels.lista.copy}
      </button>
    </div>
  </div>

  {#if day.range}
    <div class="range-summary">
      <span class="range-dates">{rangeText}</span>
      <span class="range-meta">
        {labels.lista.rangeDays.replace('{n}', String(rangeCount))} · {labels.lista.rangeUnits.replace('{n}', String(list?.total ?? 0))}
      </span>
    </div>
  {/if}

  {#if !list || (list.entries?.length ?? 0) === 0}
    <div class="clip-wrap">
      <div class="clip" aria-hidden="true"></div>
      <div class="clipboard">
        <p class="muted">{day.range ? labels.lista.rangeEmpty : labels.lista.empty}</p>
      </div>
    </div>
  {:else}
    <div class="clip-wrap">
      <div class="clip" aria-hidden="true"></div>
      <div class="clipboard">
        {#each groups as group (group.category)}
          <div class="cat">{labels.categories[group.category] ?? group.category}</div>
          <div class="prep">
            <div class="ph r">{labels.lista.quantity}</div>
            <div class="ph">{labels.lista.product}</div>
            <div class="ph">{labels.lista.measure}</div>
            <div class="ph">{labels.lista.note}</div>
            {#each group.entries as entry (entry.unitId)}
              <div class="row">
                <div class="qty">{entry.quantity}</div>
                <div class="prod">
                  <span>{entry.name}</span>
                  {#if entry.sources && entry.sources.length > 0}
                    <div class="from" title={labels.lista.origin}>
                      {#each visibleSources(entry) as source (sourceKey(source))}
                        <span class="from-item">
                          {#if source.date}<span class="from-date">{shortDate(source.date)}</span>{/if}
                          <b>{source.quantity}×</b>
                          <span class="from-name">{source.productText}</span>
                          {#if source.orderNumber}<span class="from-ref mono">#{source.orderNumber}</span>{/if}
                        </span>
                      {/each}
                      {#if entry.sources.length > sourcePreview}
                        <button class="from-toggle" onclick={() => toggleSources(entry.unitId)}>
                          {expandedSources.includes(entry.unitId) ? labels.lista.originFewer : sourceToggleText(entry)}
                        </button>
                      {/if}
                    </div>
                  {/if}
                </div>
                <div class="medida">{entry.measure ? (labels.measures[entry.measure] ?? entry.measure) : '-'}</div>
                <div class="nota">{entry.note || '-'}</div>
              </div>
            {/each}
          </div>
        {/each}

        {#if list.unresolved > 0}
          <div class="warn-block">
            <span class="stamp big">{labels.lista.unresolved}</span>
            <div class="txt">
              <b>{unresolvedTitle}</b>
              <span>{labels.lista.unresolvedHint}</span>
              {#if list.unresolvedItems && list.unresolvedItems.length > 0}
                <ul class="warn-list">
                  {#each list.unresolvedItems as item, position (position)}
                    <li>
                      <div class="warn-product">
                        <b>{item.quantity}×</b>
                        <span>{item.productText}</span>
                        <span class="stamp jam mini">{reasonText(item.reason)}</span>
                      </div>
                      {#if item.detail}
                        <div class="warn-detail">{item.detail}</div>
                      {/if}
                      {#if item.references && item.references.length > 0}
                        <div class="warn-refs">
                          {#each item.references as reference (reference)}<span class="chip mono">#{reference}</span>{/each}
                        </div>
                      {/if}
                    </li>
                  {/each}
                </ul>
              {/if}
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</section>
