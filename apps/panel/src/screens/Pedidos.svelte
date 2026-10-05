<script lang="ts">
  import { labels } from '../lib/labels'
  import { api } from '../lib/api'
  import type { PanelEntry, PanelOrder, PanelOrderDetail } from '../lib/types'
  import { day } from '../lib/day.svelte'
  import DayPicker from '../components/DayPicker.svelte'
  import SyncButton from '../components/SyncButton.svelte'

  interface Props {
    revision?: number
    role?: string
    notify?: (message: string, error?: boolean) => void
  }
  let { revision = 0, role = 'spectator', notify }: Props = $props()

  const unitPreview = 4

  let orders = $state<PanelOrder[]>([])
  let query = $state('')
  const date = $derived(day.date)

  // Lazy per-order detail: only fetched when a ticket or group opens.
  let details = $state<Record<string, PanelOrderDetail>>({})
  let loadingDetails = $state<string[]>([])
  let expanded = $state<string[]>([])
  let expandedUnits = $state<string[]>([])

  // Attach: facet filters merge matching orders, and tickets can be selected
  // by hand. Both live only in this view.
  let attachMode = $state(false)
  let filters = $state<string[]>([])
  let selected = $state<string[]>([])
  let manualGroups = $state<string[][]>([])
  let groupsDate = $state('')

  async function load () {
    const requested = day.date
    try {
      const result = await api<{ orders: PanelOrder[] }>(`/api/panel/orders?date=${requested}`)
      if (requested !== day.date) return
      orders = result.orders
      details = {}
      loadingDetails = []
      expanded = []
      expandedUnits = []
      const present = new Set(orders.map((order) => order.number))
      if (groupsDate !== requested) {
        manualGroups = []
        groupsDate = requested
      } else {
        manualGroups = manualGroups.map((group) => group.filter((number) => present.has(number))).filter((group) => group.length > 0)
      }
      selected = selected.filter((number) => present.has(number))
    } catch {
      if (requested === day.date) orders = []
    }
  }

  async function remove (order: PanelOrder) {
    try {
      await api(`/api/panel/orders/${date}/${order.number}/remove`, { method: 'POST' })
      await load()
    } catch (error) {
      notify?.((error as Error).message, true)
    }
  }

  $effect(() => {
    void revision
    void day.date
    void load()
  })

  async function loadDetails (numbers: string[]) {
    const missing = numbers.filter((number) => !details[number] && !loadingDetails.includes(number))
    if (missing.length === 0) return
    loadingDetails = [...loadingDetails, ...missing]
    try {
      const params = new URLSearchParams({ date: day.date, numbers: missing.join(',') })
      const result = await api<{ orders: PanelOrderDetail[] }>(`/api/panel/order-details?${params}`)
      const next = { ...details }
      for (const detail of result.orders) next[detail.number] = detail
      details = next
    } catch (error) {
      notify?.((error as Error).message, true)
    } finally {
      loadingDetails = loadingDetails.filter((number) => !missing.includes(number))
    }
  }

  function toggleTicket (key: string, numbers: string[]) {
    if (expanded.includes(key)) {
      expanded = expanded.filter((item) => item !== key)
      return
    }
    expanded = [...expanded, key]
    void loadDetails(numbers)
  }

  function toggleUnits (key: string) {
    expandedUnits = expandedUnits.includes(key)
      ? expandedUnits.filter((item) => item !== key)
      : [...expandedUnits, key]
  }

  function measureText (entry: PanelEntry) {
    if (!entry.measure) return ''
    const measure = labels.measures[entry.measure] ?? entry.measure
    return measure === '-' ? '' : measure
  }

  function statusText (order: PanelOrder) {
    return order.status === 'notified' ? labels.pedidos.notified : labels.pedidos.pending
  }

  const searched = $derived.by(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return orders
    return orders.filter((order) =>
      order.number.toLowerCase().includes(needle) ||
      order.text.toLowerCase().includes(needle) ||
      (order.note ?? '').toLowerCase().includes(needle))
  })

  const facetGroups = $derived.by(() => {
    const kindOrder = ['breakfast', 'add_on', 'color', 'reason', 'other']
    const map = new Map<string, Map<string, number>>()
    for (const order of searched) {
      for (const facet of order.facets ?? []) {
        if (!map.has(facet.kind)) map.set(facet.kind, new Map())
        const values = map.get(facet.kind)!
        values.set(facet.value, (values.get(facet.value) ?? 0) + 1)
      }
    }
    const kinds = [
      ...kindOrder.filter((kind) => map.has(kind)),
      ...[...map.keys()].filter((kind) => !kindOrder.includes(kind)),
    ]
    return kinds.map((kind) => ({
      kind,
      label: labels.pedidos.facetKinds[kind] ?? kind,
      values: [...map.get(kind)!.entries()]
        .sort((a, b) => a[0].localeCompare(b[0], 'es'))
        .map(([value, count]) => ({ value, count })),
    }))
  })

  function facetKey (kind: string, value: string) {
    return `${kind}|${value}`
  }

  function splitFacet (key: string) {
    const at = key.indexOf('|')
    return { kind: key.slice(0, at), value: key.slice(at + 1) }
  }

  function toggleFilter (kind: string, value: string) {
    const key = facetKey(kind, value)
    filters = filters.includes(key) ? filters.filter((item) => item !== key) : [...filters, key]
  }

  function matchesFilters (order: PanelOrder) {
    return filters.every((key) => {
      const { kind, value } = splitFacet(key)
      return (order.facets ?? []).some((facet) => facet.kind === kind && facet.value === value)
    })
  }

  function toggleSelect (number: string) {
    selected = selected.includes(number)
      ? selected.filter((item) => item !== number)
      : [...selected, number]
  }

  function attachSelected () {
    if (selected.length < 2) {
      notify?.(labels.pedidos.attachTwo)
      return
    }
    const group = selected
    manualGroups = [...manualGroups, group]
    selected = []
    attachMode = false
    expanded = [...expanded, `group:${manualGroups.length - 1}`]
    void loadDetails(group)
  }

  function separate (index: number) {
    manualGroups = manualGroups.filter((_, position) => position !== index)
  }

  function mergedEntries (numbers: string[]): PanelEntry[] {
    const map = new Map<string, PanelEntry>()
    for (const number of numbers) {
      const detail = details[number]
      if (!detail) continue
      for (const entry of detail.entries) {
        const existing = map.get(entry.unitId)
        if (existing) {
          map.set(entry.unitId, { ...existing, quantity: existing.quantity + entry.quantity })
        } else {
          map.set(entry.unitId, { ...entry })
        }
      }
    }
    return [...map.values()]
  }

  const attachedNumbers = $derived(new Set(manualGroups.flat()))

  const filterMatches = $derived(
    filters.length === 0
      ? []
      : searched.filter((order) => !attachedNumbers.has(order.number) && matchesFilters(order)),
  )

  const filterTotal = $derived(
    filterMatches.reduce((total, order) => {
      const detail = details[order.number]
      return total + (detail ? detail.entries.reduce((sum, entry) => sum + entry.quantity, 0) : 0)
    }, 0),
  )

  const plain = $derived(
    searched.filter((order) => !attachedNumbers.has(order.number) && !(filters.length > 0 && matchesFilters(order))),
  )

  const manualTickets = $derived.by(() => {
    const searching = query.trim() !== ''
    const visible = new Set(searched.map((order) => order.number))
    return manualGroups
      .map((numbers, index) => ({
        index,
        numbers,
        orders: numbers
          .map((number) => orders.find((order) => order.number === number))
          .filter((order): order is PanelOrder => Boolean(order)),
      }))
      .filter((group) => group.orders.length > 0 && (!searching || group.numbers.some((number) => visible.has(number))))
  })

  const filterKey = $derived(`filter:${filters.join(',')}`)

  function mergedTotal (numbers: string[]) {
    return numbers.reduce((total, number) => {
      const detail = details[number]
      return total + (detail ? detail.entries.reduce((sum, entry) => sum + entry.quantity, 0) : 0)
    }, 0)
  }

  function hasDetails (numbers: string[]) {
    return numbers.some((number) => Boolean(details[number]))
  }

  function previewMembers (orders: PanelOrder[]) {
    return orders.slice(0, 10)
  }

  // Unique color/motivo values of a merged ticket, so the group head still
  // shows the personalization its members share.
  function uniqueAnnotation (orders: PanelOrder[], field: 'color' | 'reason') {
    const values = new Set<string>()
    for (const order of orders) {
      const value = order[field]
      if (value) values.add(value)
    }
    return [...values]
  }
</script>

{#snippet unitList (key: string, entries: PanelEntry[], total: number)}
  {#if entries.length > 0}
    <div class="t-units">
      <span class="t-units-head">{labels.pedidos.units.replace('{n}', String(total))}</span>
      <ul class="t-units-list">
        {#each (expandedUnits.includes(key) ? entries : entries.slice(0, unitPreview)) as entry (entry.unitId)}
          <li>
            <b>{entry.quantity}</b>
            <span>{entry.name}</span>
            {#if measureText(entry)}<span class="muted">{measureText(entry)}</span>{/if}
          </li>
        {/each}
      </ul>
      {#if entries.length > unitPreview}
        <button class="btn btn-ghost btn-sm" onclick={() => toggleUnits(key)}>
          {expandedUnits.includes(key) ? labels.pedidos.showFewerUnits : labels.pedidos.showAllUnits}
        </button>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet annotationChips (order: PanelOrder)}
  {#if order.color}<span class="chip mini tone-color">{labels.pedidos.color}: {order.color}</span>{/if}
  {#if order.reason}<span class="chip mini tone-reason">{labels.pedidos.reason}: {order.reason}</span>{/if}
{/snippet}

{#snippet groupAnnotationChips (orders: PanelOrder[])}
  {#each uniqueAnnotation(orders, 'color') as value (value)}<span class="chip mini tone-color">{labels.pedidos.color}: {value}</span>{/each}
  {#each uniqueAnnotation(orders, 'reason') as value (value)}<span class="chip mini tone-reason">{labels.pedidos.reason}: {value}</span>{/each}
{/snippet}

{#snippet lineList (numbers: string[])}
  {#each numbers as number (number)}
    {#if details[number]}
      <div class="t-lines">
        {#each details[number].lines as line, position (position)}
          <div class="t-line-row">
            <span class="t-line-qty">x{line.quantity}</span>
            <span class="t-line-name">{line.productText}</span>
            {#if line.options.length > 0}
              <span class="t-line-options">
                {#each line.options as option (option)}<span class="chip mini">{option}</span>{/each}
              </span>
            {/if}
            {#if line.status !== 'resolved'}
              <span class="stamp jam mini">{labels.lista.unresolved}</span>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {/each}
{/snippet}

<section class="screen">
  <div class="toolbar">
    <DayPicker />
    <SyncButton {revision} date={day.date} {notify} onsynced={load} />
    <button
      class="btn btn-kraft"
      class:active={attachMode}
      aria-pressed={attachMode}
      title={labels.pedidos.attachTitle}
      onclick={() => {
        attachMode = !attachMode
        if (!attachMode) {
          filters = []
          selected = []
        }
      }}
    >
      <svg viewBox="0 0 24 24"><path d="M9 4h6v3a3 3 0 0 0 3 3h3v6h-3a3 3 0 0 0-3 3v1H9v-1a3 3 0 0 0-3-3H3V9h3a3 3 0 0 0 3-3z" /></svg>
      {labels.pedidos.attach}
      {#if filters.length + selected.length > 0}
        <span class="badge">{filters.length + selected.length}</span>
      {/if}
    </button>
    <div class="search">
      <svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
      <input class="input" bind:value={query} placeholder={labels.pedidos.search} />
    </div>
  </div>

  {#if attachMode}
    <div class="attach-panel">
      <div class="attach-head">
        <span class="attach-hint">{labels.pedidos.attachHint}</span>
        {#if filters.length > 0}
          <button class="btn btn-ghost btn-sm" onclick={() => (filters = [])}>{labels.pedidos.attachClear}</button>
        {/if}
      </div>
      {#if facetGroups.length === 0}
        <p class="muted small">{labels.pedidos.attachEmpty}</p>
      {:else}
        {#each facetGroups as group (group.kind)}
          <div class="attach-group">
            <span class="attach-group-label">{group.label}</span>
            <div class="attach-values">
              {#each group.values as item (item.value)}
                <button
                  class="facet"
                  class:on={filters.includes(facetKey(group.kind, item.value))}
                  onclick={() => toggleFilter(group.kind, item.value)}
                >
                  {item.value}
                  <span class="count">{item.count}</span>
                </button>
              {/each}
            </div>
          </div>
        {/each}
      {/if}
    </div>
  {/if}

  {#if orders.length === 0}
    <p class="muted">{labels.pedidos.empty}</p>
  {:else if searched.length === 0}
    <p class="muted">{labels.pedidos.noResults}</p>
  {:else if manualTickets.length === 0 && filterMatches.length === 0 && plain.length === 0}
    <p class="muted">{labels.pedidos.noFilterResults}</p>
  {:else}
    {#if filters.length > 0 && filterMatches.length === 0}
      <p class="muted small">{labels.pedidos.noFilterResults}</p>
    {/if}
    <div class="tickets">
      {#each manualTickets as group (group.index)}
        <article class="ticket merged">
          <div class="t-head">
            <button class="t-toggle" aria-expanded={expanded.includes(`group:${group.index}`)} onclick={() => toggleTicket(`group:${group.index}`, group.numbers)}>
              <span class="t-num">{labels.pedidos.attachedGroup.replace('{n}', String(group.orders.length))}</span>
              <span class="t-members">
                {#each previewMembers(group.orders) as member (member.number)}<span class="chip mono">#{member.number}</span>{/each}
                {#if group.orders.length > 10}<span class="chip mono">+{group.orders.length - 10}</span>{/if}
              </span>
              {@render groupAnnotationChips(group.orders)}
              <span class="spacer"></span>
              <span class="t-total">
                {#if hasDetails(group.numbers)}{mergedTotal(group.numbers)} {labels.pedidos.unitsWord}{:else}{group.orders.length} {labels.pedidos.ordersWord}{/if}
              </span>
              <svg class="t-chevron" class:on={expanded.includes(`group:${group.index}`)} viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg>
            </button>
            <button class="btn btn-ghost btn-sm" onclick={() => separate(group.index)}>{labels.pedidos.separate}</button>
          </div>

          {#if expanded.includes(`group:${group.index}`)}
            <div class="t-detail">
              {#if group.numbers.some((number) => loadingDetails.includes(number))}
                <p class="muted small">{labels.pedidos.loadingDetail}</p>
              {/if}
              {#each group.orders as member (member.number)}
                <div class="t-member">
                  <div class="t-member-head">
                    {#if attachMode}
                      <label class="t-pick" title={labels.pedidos.selectTicket}>
                        <input type="checkbox" checked={selected.includes(member.number)} onchange={() => toggleSelect(member.number)} />
                      </label>
                    {/if}
                    <span class="t-num small">#{member.number}</span>
                    <span class="stamp {member.status === 'notified' ? 'herb' : 'honey'}">{statusText(member)}</span>
                    {@render annotationChips(member)}
                    <span class="t-line">{member.text}</span>
                  </div>
                  {#if member.note}
                    <p class="t-note"><span class="t-note-label">{labels.pedidos.note}</span> {member.note}</p>
                  {/if}
                  {@render lineList([member.number])}
                </div>
              {/each}
              {@render unitList(`group:${group.index}`, mergedEntries(group.numbers), mergedTotal(group.numbers))}
            </div>
          {/if}
        </article>
      {/each}

      {#if filterMatches.length > 0}
        <article class="ticket merged filter">
          <div class="t-head">
            <button class="t-toggle" aria-expanded={expanded.includes(filterKey)} onclick={() => toggleTicket(filterKey, filterMatches.map((order) => order.number))}>
              <span class="t-num">{labels.pedidos.attachedByFilter}</span>
              <span class="t-members">
                {#each previewMembers(filterMatches) as member (member.number)}<span class="chip mono">#{member.number}</span>{/each}
                {#if filterMatches.length > 10}<span class="chip mono">+{filterMatches.length - 10}</span>{/if}
              </span>
              {@render groupAnnotationChips(filterMatches)}
              <span class="spacer"></span>
              <span class="t-total">
                {#if hasDetails(filterMatches.map((order) => order.number))}{filterTotal} {labels.pedidos.unitsWord}{:else}{filterMatches.length} {labels.pedidos.ordersWord}{/if}
              </span>
              <svg class="t-chevron" class:on={expanded.includes(filterKey)} viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg>
            </button>
          </div>

          {#if expanded.includes(filterKey)}
            <div class="t-detail">
              {#if filterMatches.some((order) => loadingDetails.includes(order.number))}
                <p class="muted small">{labels.pedidos.loadingDetail}</p>
              {/if}
              {#each filterMatches as member (member.number)}
                <div class="t-member">
                  <div class="t-member-head">
                    {#if attachMode}
                      <label class="t-pick" title={labels.pedidos.selectTicket}>
                        <input type="checkbox" checked={selected.includes(member.number)} onchange={() => toggleSelect(member.number)} />
                      </label>
                    {/if}
                    <span class="t-num small">#{member.number}</span>
                    <span class="stamp {member.status === 'notified' ? 'herb' : 'honey'}">{statusText(member)}</span>
                    {@render annotationChips(member)}
                    <span class="t-line">{member.text}</span>
                  </div>
                  {#if member.note}
                    <p class="t-note"><span class="t-note-label">{labels.pedidos.note}</span> {member.note}</p>
                  {/if}
                  {@render lineList([member.number])}
                </div>
              {/each}
              {@render unitList(filterKey, mergedEntries(filterMatches.map((order) => order.number)), filterTotal)}
            </div>
          {/if}
        </article>
      {/if}

      {#each plain as order (order.number)}
        {@const key = `order:${order.number}`}
        <article class="ticket">
          <div class="t-head">
            {#if attachMode}
              <label class="t-pick" title={labels.pedidos.selectTicket}>
                <input type="checkbox" checked={selected.includes(order.number)} onchange={() => toggleSelect(order.number)} />
              </label>
            {/if}
            <button class="t-toggle" aria-expanded={expanded.includes(key)} onclick={() => toggleTicket(key, [order.number])}>
              <span class="t-num">#{order.number}</span>
              <span class="stamp {order.status === 'notified' ? 'herb' : 'honey'}">{statusText(order)}</span>
              {@render annotationChips(order)}
              <span class="t-line">{order.text}</span>
              <span class="spacer"></span>
              <span class="t-total">{labels.pedidos.units.replace('{n}', String(order.units))}</span>
              <svg class="t-chevron" class:on={expanded.includes(key)} viewBox="0 0 24 24"><path d="M6 9l6 6 6-6" /></svg>
            </button>
            {#if role === 'host'}
              <button class="btn btn-danger btn-sm" onclick={() => remove(order)}>{labels.pedidos.remove}</button>
            {/if}
          </div>

          {#if order.note}
            <p class="t-note"><span class="t-note-label">{labels.pedidos.note}</span> {order.note}</p>
          {/if}

          {#if expanded.includes(key)}
            {@const detail = details[order.number]}
            <div class="t-detail">
              {#if loadingDetails.includes(order.number) && !detail}
                <p class="muted small">{labels.pedidos.loadingDetail}</p>
              {:else if detail}
                {@render lineList([order.number])}
                {@render unitList(key, detail.entries, detail.entries.reduce((sum, entry) => sum + entry.quantity, 0))}
              {/if}
            </div>
          {/if}
        </article>
      {/each}
    </div>
  {/if}

  {#if attachMode && selected.length > 0}
    <div class="attach-bar">
      <span class="muted small">{labels.pedidos.selectedCount.replace('{n}', String(selected.length))}</span>
      <span class="spacer"></span>
      <button class="btn btn-ghost btn-sm" onclick={() => (selected = [])}>{labels.pedidos.attachCancel}</button>
      <button class="btn btn-primary btn-sm" disabled={selected.length < 2} onclick={attachSelected}>{labels.pedidos.attachSelected}</button>
    </div>
  {/if}
</section>
