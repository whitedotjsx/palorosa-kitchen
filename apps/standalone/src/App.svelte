<script lang="ts">
  import {
    aggregateUnits,
    appLabels,
    formatKitchenTemplateHtml,
    formatListCsv,
    formatListText,
    groupEntriesByCategory,
    indexCatalog,
    listLabels,
    measureLabels,
    normalizeName,
    resolveLines,
    unitCategoryLabels,
    unresolvedReasonLabels,
    warningLabels,
    type Catalog,
    type ParsedOrderLine,
    type SkippedText,
  } from '@palorosa-kitchen/core'
  import { catalog, catalogMeta } from 'virtual:catalog-snapshot'
  import palorosaLogo from '@palorosa-kitchen/core/assets/palorosa-logo.png'
  import { importOrdersFile } from './lib/import-orders'
  import { loadProductMappings, saveProductMappings } from './lib/storage'

  const snapshot = catalog as Catalog
  const index = indexCatalog(snapshot)
  const catalogDate = new Intl.DateTimeFormat('es-CO', {
    dateStyle: 'medium',
    timeZone: 'America/Bogota',
  }).format(new Date(catalogMeta.mtime))

  let deliveryDate = $state(bogotaToday())
  let sourceName = $state('')
  let lines = $state<ParsedOrderLine[]>([])
  let skipped = $state<SkippedText[]>([])
  let statusMessage = $state('')
  let reading = $state(false)
  let pdfFallback = $state(false)
  let unreadableStreams = $state(0)
  let dragging = $state(false)
  let copied = $state(false)
  let mappings = $state<Record<string, string>>(loadProductMappings())
  let fileInput: HTMLInputElement | undefined = $state()
  let published = $state(false)
  let publishError = $state('')

  const resolved = $derived(resolveLines(lines, index, { productMappings: mappings }))
  const list = $derived(aggregateUnits(resolved, index))
  const groups = $derived(groupEntriesByCategory(list))
  const hasOrders = $derived(lines.length > 0)
  const orderCount = $derived(
    new Set(lines.map(line => line.orderNumber).filter((value): value is string => Boolean(value))).size
  )
  const kitchenProducts = $derived(
    snapshot.products
      .filter(product => product.isKitchen && product.active)
      .sort((a, b) => a.name.localeCompare(b.name, 'es'))
  )
  const savedMappings = $derived(
    Object.entries(mappings).map(([text, productId]) => ({
      text,
      productId,
      productName: index.productsById.get(productId)?.name ?? productId,
    }))
  )
  const ignoredEntries = $derived([
    ...list.ignoredProducts.map(entry => ({ ...entry, kind: 'product' })),
    ...list.ignoredUnits.map(entry => ({ ...entry, kind: 'unit' })),
  ])

  function bogotaToday (): string {
    return new Intl.DateTimeFormat('en-CA', { timeZone: 'America/Bogota' }).format(new Date())
  }

  async function handleFiles (files: FileList | null): Promise<void> {
    const file = files?.[0]
    if (!file) return
    reading = true
    statusMessage = ''
    try {
      const result = await importOrdersFile(file, snapshot.labelParsing)
      lines = result.lines
      skipped = result.skipped
      pdfFallback = result.pdfFallback
      unreadableStreams = result.unreadableStreams
      sourceName = result.source
      if (result.deliveryDate) deliveryDate = result.deliveryDate
      const resolvedResult = resolveLines(result.lines, index, { productMappings: mappings })
      const computed = aggregateUnits(resolvedResult, index)
      void publishList(computed, deliveryDate, result.lines)
    } catch (error) {
      lines = []
      skipped = []
      pdfFallback = false
      unreadableStreams = 0
      sourceName = file.name
      statusMessage = error instanceof Error ? error.message : appLabels.reading
    } finally {
      reading = false
    }
  }

  async function publishList (
    computed: ReturnType<typeof aggregateUnits>,
    date: string,
    resultLines: ParsedOrderLine[]
  ): Promise<void> {
    published = false
    publishError = ''
    if (!location.protocol.startsWith('http')) return
    try {
      const orderCountValue = new Set(
        resultLines.map(line => line.orderNumber).filter((value): value is string => Boolean(value))
      ).size
      const response = await fetch('/api/list', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          deliveryDate: date,
          source: sourceName,
          orderCount: orderCountValue,
          entries: computed.entries,
          unresolved: computed.unresolved,
        }),
      })
      if (!response.ok) throw new Error(String(response.status))
      published = true
    } catch {
      publishError = appLabels.publishFailed
    }
  }

  function onDrop (event: DragEvent): void {
    event.preventDefault()
    dragging = false
    void handleFiles(event.dataTransfer?.files ?? null)
  }

  function clearAll (): void {
    lines = []
    skipped = []
    sourceName = ''
    statusMessage = ''
    pdfFallback = false
    unreadableStreams = 0
    published = false
    publishError = ''
  }

  function mapProduct (productText: string, productId: string): void {
    if (!productId) return
    mappings = { ...mappings, [normalizeName(productText)]: productId }
    saveProductMappings(mappings)
  }

  function clearMapping (text: string): void {
    const next = { ...mappings }
    delete next[text]
    mappings = next
    saveProductMappings(next)
  }

  async function printList (): Promise<void> {
    if (location.protocol.startsWith('http')) {
      try {
        const response = await fetch(`/api/pdf?date=${encodeURIComponent(deliveryDate)}`)
        const contentType = response.headers.get('content-type') ?? ''
        if (response.ok && contentType.includes('application/pdf')) {
          const blob = await response.blob()
          const url = URL.createObjectURL(blob)
          const anchor = document.createElement('a')
          anchor.href = url
          anchor.download = `cocina-palorosa-${deliveryDate}.pdf`
          anchor.click()
          URL.revokeObjectURL(url)
          return
        }
      } catch {
        // No kitchen server on this origin: fall back to the browser print dialog.
      }
    }
    const frame = document.createElement('iframe')
    frame.style.position = 'fixed'
    frame.style.width = '0'
    frame.style.height = '0'
    frame.style.border = '0'
    document.body.appendChild(frame)
    frame.srcdoc = formatKitchenTemplateHtml(list, { date: deliveryDate, logo: palorosaLogo })
    frame.onload = () => {
      frame.contentWindow?.focus()
      frame.contentWindow?.print()
      setTimeout(() => frame.remove(), 2000)
    }
  }

  async function copyText (): Promise<void> {
    const text = formatListText(list, { date: deliveryDate })
    try {
      await navigator.clipboard.writeText(text)
      copied = true
      setTimeout(() => {
        copied = false
      }, 1500)
    } catch {
      statusMessage = appLabels.copy
    }
  }

  function downloadCsv (): void {
    const csv = formatListCsv(list, { includeUnresolved: true })
    const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `lista-cocina-${deliveryDate}.csv`
    anchor.click()
    URL.revokeObjectURL(url)
  }
</script>

<main>
  <header>
    <h1>{appLabels.title}</h1>
    <p class="meta">
      {appLabels.snapshotDate}: {catalogDate} · {appLabels.catalog} v{snapshot.schemaVersion}
    </p>
  </header>

  <section class="card">
    <h2>{appLabels.importTitle}</h2>
    <div
      class="dropzone"
      class:active={dragging}
      role="button"
      tabindex="0"
      aria-label={appLabels.chooseFile}
      onclick={() => fileInput?.click()}
      onkeydown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          fileInput?.click()
        }
      }}
      ondragover={(event) => {
        event.preventDefault()
        dragging = true
      }}
      ondragleave={() => (dragging = false)}
      ondrop={onDrop}
    >
      <p>{appLabels.dropHint}</p>
      <span class="choose">{appLabels.chooseFile}</span>
      <input
        bind:this={fileInput}
        type="file"
        accept=".pdf,.json,.xlsx,.csv,application/pdf,application/json,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv"
        hidden
        onchange={(event) => void handleFiles(event.currentTarget.files)}
      />
    </div>
    <div class="controls">
      <label>
        {appLabels.deliveryDate}
        <input type="date" bind:value={deliveryDate} />
      </label>
      {#if hasOrders || statusMessage}
        <button type="button" onclick={clearAll}>{appLabels.clear}</button>
      {/if}
    </div>
    {#if reading}<p class="hint">{appLabels.reading}</p>{/if}
    {#if statusMessage}<p class="error">{statusMessage}</p>{/if}
    {#if pdfFallback}<p class="hint">{appLabels.pdfFallback}</p>{/if}
    {#if unreadableStreams > 0}
      <p class="hint">{appLabels.unreadableStreams}: {unreadableStreams}</p>
    {/if}
    {#if hasOrders}
      <p class="summary">
        {appLabels.ordersCount}: {orderCount} · {appLabels.linesCount}: {lines.length} · {sourceName}
        {#if published}· {appLabels.published}{/if}
        {#if publishError}· {publishError}{/if}
      </p>
    {/if}
  </section>

  {#if savedMappings.length > 0}
    <details class="card">
      <summary>{appLabels.mappingsTitle} ({savedMappings.length})</summary>
      <ul>
        {#each savedMappings as mapping (mapping.text)}
          <li>
            <span class="muted">{mapping.text}</span> → {mapping.productName}
            <button type="button" class="link" onclick={() => clearMapping(mapping.text)}>
              {appLabels.mappingClear}
            </button>
          </li>
        {/each}
      </ul>
    </details>
  {/if}

  {#if !hasOrders}
    <p class="empty">{appLabels.noOrders}</p>
  {:else}
    <div class="actions">
      <button type="button" onclick={printList}>{appLabels.print}</button>
      <button type="button" onclick={() => void copyText()}>
        {copied ? appLabels.copied : appLabels.copy}
      </button>
      <button type="button" onclick={downloadCsv}>{appLabels.downloadCsv}</button>
    </div>

    <section class="card">
      <h2>{listLabels.title} · {deliveryDate}</h2>
      {#if groups.length === 0}
        <p>{listLabels.empty}</p>
      {/if}
      {#each groups as group (group.category)}
        <h3>{unitCategoryLabels[group.category]}</h3>
        <table>
          <thead>
            <tr>
              <th>{listLabels.quantity}</th>
              <th>{listLabels.item}</th>
              <th>{listLabels.measure}</th>
              <th>{listLabels.note}</th>
              <th>{appLabels.references}</th>
            </tr>
          </thead>
          <tbody>
            {#each group.entries as entry (entry.unitId)}
              <tr>
                <td class="qty">{entry.quantity}</td>
                <td>{entry.name}</td>
                <td class="muted">{measureLabels[entry.measure]}</td>
                <td class="muted">{entry.note ?? ''}</td>
                <td class="muted">{entry.references.join(', ')}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/each}
    </section>

    {#if list.unresolved.length > 0}
      <section class="card warning">
        <h2>{appLabels.unresolvedTitle}</h2>
        <p class="hint">{appLabels.mappingHint}</p>
        <table>
          <thead>
            <tr>
              <th>{listLabels.quantity}</th>
              <th>{listLabels.item}</th>
              <th>Detalle</th>
              <th>{appLabels.references}</th>
              <th>{appLabels.mapToProduct}</th>
            </tr>
          </thead>
          <tbody>
            {#each list.unresolved as entry (`${entry.reason}-${entry.productText}-${entry.detail ?? ''}`)}
              <tr>
                <td class="qty">{entry.quantity}</td>
                <td>{entry.productText}</td>
                <td class="muted">
                  {unresolvedReasonLabels[entry.reason]}{entry.detail ? ` · ${entry.detail}` : ''}
                </td>
                <td class="muted">{entry.references.join(', ')}</td>
                <td>
                  {#if entry.reason === 'unknown_product' || entry.reason === 'missing_recipe'}
                    <select
                      onchange={(event) => mapProduct(entry.productText, event.currentTarget.value)}
                    >
                      <option value="">{appLabels.selectProduct}</option>
                      {#each kitchenProducts as product (product.id)}
                        <option value={product.id}>{product.name}</option>
                      {/each}
                    </select>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>
    {/if}

    {#if list.warnings.length > 0}
      <details class="card">
        <summary>{appLabels.warningsTitle} ({list.warnings.length})</summary>
        <ul>
          {#each list.warnings as warning (warning.raw)}
            <li>
              {warning.quantity} × {warning.productText} — {warningLabels.missing_quantity}
              <span class="muted">({warning.references.join(', ')})</span>
            </li>
          {/each}
        </ul>
      </details>
    {/if}

    {#if ignoredEntries.length > 0}
      <details class="card">
        <summary>{appLabels.ignoredProductsTitle} ({ignoredEntries.length})</summary>
        <ul>
          {#each ignoredEntries as entry (`${entry.kind}-${entry.reason}-${entry.id}`)}
            <li>
              {entry.quantity} × {entry.name}
              <span class="muted">({entry.references.join(', ')})</span>
            </li>
          {/each}
        </ul>
      </details>
    {/if}

    {#if skipped.length > 0}
      <details class="card">
        <summary>{appLabels.skippedTitle} ({skipped.length})</summary>
        <ul>
          {#each skipped as entry, index (`${index}-${entry.raw}`)}
            <li><span class="muted">{entry.raw}</span></li>
          {/each}
        </ul>
      </details>
    {/if}
  {/if}
</main>

<style>
  :global(body) {
    margin: 0;
    background: #f6f5f2;
    color: #1a1a1a;
    font-family: system-ui, -apple-system, 'Segoe UI', sans-serif;
  }

  main {
    max-width: 960px;
    margin: 0 auto;
    padding: 24px 16px 64px;
  }

  header h1 {
    font-size: 24px;
    margin: 0;
  }

  .meta,
  .muted {
    color: #6b6b6b;
    font-size: 13px;
  }

  .meta {
    margin: 4px 0 20px;
  }

  .card {
    background: #fff;
    border: 1px solid #e2e0da;
    border-radius: 10px;
    padding: 16px;
    margin-bottom: 16px;
  }

  .card h2 {
    font-size: 16px;
    margin: 0 0 12px;
  }

  .card h3 {
    font-size: 13px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #8a1c1c;
    margin: 18px 0 6px;
  }

  .dropzone {
    border: 2px dashed #c9c5bb;
    border-radius: 10px;
    padding: 20px;
    text-align: center;
    cursor: pointer;
  }

  .dropzone:focus-visible {
    outline: 2px solid #8a1c1c;
    outline-offset: 2px;
  }

  .choose {
    display: inline-block;
    padding: 7px 14px;
    border-radius: 8px;
    background: #8a1c1c;
    color: #fff;
  }

  .dropzone.active {
    border-color: #8a1c1c;
    background: #fdf6f5;
  }

  .dropzone p {
    margin: 0 0 10px;
    color: #555;
  }

  .controls {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-top: 12px;
  }

  .controls label {
    display: flex;
    gap: 8px;
    align-items: center;
    font-size: 14px;
  }

  button {
    font: inherit;
    padding: 7px 14px;
    border-radius: 8px;
    border: 1px solid #8a1c1c;
    background: #8a1c1c;
    color: #fff;
    cursor: pointer;
  }

  button.link {
    border: 0;
    background: none;
    color: #8a1c1c;
    padding: 0 4px;
    text-decoration: underline;
  }

  .actions {
    display: flex;
    gap: 8px;
    margin-bottom: 16px;
  }

  table {
    width: 100%;
    border-collapse: collapse;
  }

  th {
    text-align: left;
    font-size: 11px;
    text-transform: uppercase;
    color: #777;
    border-bottom: 1px solid #ccc;
    padding: 2px 6px 4px 0;
  }

  td {
    padding: 5px 6px 5px 0;
    border-bottom: 1px solid #efeee9;
    vertical-align: top;
  }

  td.qty {
    font-weight: 600;
    width: 56px;
  }

  .summary {
    margin: 12px 0 0;
    font-size: 14px;
  }

  .hint {
    color: #6b6b6b;
    margin: 10px 0 0;
    font-size: 14px;
  }

  .error {
    color: #8a1c1c;
    margin: 10px 0 0;
  }

  .empty {
    color: #6b6b6b;
    text-align: center;
    padding: 32px 0;
  }

  .warning {
    border-color: #e3cf89;
  }

  summary {
    cursor: pointer;
    font-weight: 600;
  }

  ul {
    margin: 10px 0 0;
    padding-left: 18px;
  }

  li {
    margin-bottom: 4px;
  }

  select {
    font: inherit;
    max-width: 260px;
  }
</style>
