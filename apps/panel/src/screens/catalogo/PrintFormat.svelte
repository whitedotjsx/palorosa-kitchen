<script lang="ts">
	import { onMount } from 'svelte'
	import type { PrintTemplate, TemplateBlock } from '@palorosa-kitchen/core'
	import { editor, unitName } from '../../lib/catalog.svelte'
	import { defaultTemplate, loadTemplate, printSheet, saveTemplate, sheetHtml } from '../../lib/print'
	import type { PanelList } from '../../lib/types'
	import UnitSelect from './UnitSelect.svelte'

	interface Props {
		notify?: ((message: string, error?: boolean) => void) | undefined
	}
	let { notify }: Props = $props()

	type Side = 'left' | 'right'

	let template = $state<PrintTemplate>(defaultTemplate())
	let custom = $state(false)
	let saved = $state('')
	let loading = $state(true)
	let busy = $state(false)
	let pick = $state<Record<string, string>>({})

	const dirty = $derived(JSON.stringify(template) !== saved)

	onMount(async () => {
		const result = await loadTemplate()
		template = result.template
		custom = result.custom
		saved = JSON.stringify(result.template)
		loading = false
	})

	/** Every unit id mapped somewhere in the sheet. */
	const mapped = $derived.by(() => {
		const ids = new Set<string>()
		for (const block of [...template.left, ...template.right]) {
			for (const id of block.unitIds ?? []) ids.add(id)
			for (const row of block.subRows ?? []) for (const id of row.unitIds ?? []) ids.add(id)
		}
		return ids
	})

	/** Active kitchen units with no row: they print in the "OTROS" section. */
	const unmapped = $derived(
		(editor.catalog?.units ?? [])
			.filter((unit) => unit.isKitchen && unit.active && !mapped.has(unit.id))
			.sort((a, b) => a.name.localeCompare(b.name)),
	)

	const missing = $derived([...mapped].filter((id) => !editor.catalog?.units.some((unit) => unit.id === id)))

	function slug (text: string): string {
		return text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'fila'
	}

	function uniqueKey (label: string): string {
		const taken = new Set<string>()
		for (const block of [...template.left, ...template.right]) {
			taken.add(block.key)
			for (const row of block.subRows ?? []) taken.add(row.key)
		}
		const base = slug(label)
		let key = base
		for (let n = 2; taken.has(key); n++) key = `${base}-${n}`
		return key
	}

	function addBlock (side: Side) {
		template[side].push({ key: uniqueKey('nueva fila'), label: 'NUEVA FILA', unitIds: [] })
	}

	function removeBlock (side: Side, index: number) {
		template[side].splice(index, 1)
	}

	function moveBlock (side: Side, index: number, delta: number) {
		const list = template[side]
		const target = index + delta
		if (target < 0 || target >= list.length) return
		const [block] = list.splice(index, 1)
		list.splice(target, 0, block!)
	}

	function switchSide (side: Side, index: number) {
		const [block] = template[side].splice(index, 1)
		template[side === 'left' ? 'right' : 'left'].push(block!)
	}

	function addSubRow (block: TemplateBlock) {
		// A block with sub-rows prints its own ids nowhere, so move them to the first sub-row.
		block.subRows ??= []
		if (block.subRows.length === 0 && (block.unitIds?.length ?? 0) > 0) {
			block.subRows.push({ key: uniqueKey(block.label), label: block.label, unitIds: block.unitIds ?? [] })
			block.unitIds = []
		}
		block.subRows.push({ key: uniqueKey(`${block.label} sub`), label: 'SUB', unitIds: [] })
	}

	function removeSubRow (block: TemplateBlock, index: number) {
		block.subRows?.splice(index, 1)
		if (block.subRows && block.subRows.length === 0) delete block.subRows
	}

	function addUnit (holder: { unitIds?: string[] }, key: string) {
		const id = pick[key]
		if (!id) return
		holder.unitIds ??= []
		if (!holder.unitIds.includes(id)) holder.unitIds.push(id)
		pick[key] = ''
	}

	function removeUnit (holder: { unitIds?: string[] }, id: string) {
		holder.unitIds = (holder.unitIds ?? []).filter((value) => value !== id)
	}

	async function save () {
		busy = true
		try {
			const plain = JSON.parse(JSON.stringify(template)) as PrintTemplate
			await saveTemplate(plain)
			saved = JSON.stringify(plain)
			custom = true
			notify?.('Formato guardado')
		} catch (error) {
			notify?.((error as Error).message, true)
		} finally {
			busy = false
		}
	}

	async function restore () {
		if (!window.confirm('¿Volver al formato original? Se pierden los cambios guardados.')) return
		busy = true
		try {
			await saveTemplate(null)
			template = defaultTemplate()
			saved = JSON.stringify(template)
			custom = false
			notify?.('Formato original restaurado')
		} catch (error) {
			notify?.((error as Error).message, true)
		} finally {
			busy = false
		}
	}

	function discard () {
		template = JSON.parse(saved) as PrintTemplate
	}

	/** Sample list: one of every mapped and unmapped unit, so every row shows a number. */
	function sampleList (): PanelList {
		const units = (editor.catalog?.units ?? []).filter((unit) => unit.isKitchen && unit.active)
		return {
			entries: units.map((unit) => ({
				unitId: unit.id,
				name: unit.name,
				measure: unit.measure,
				category: unit.category,
				note: '',
				quantity: 1,
			})),
			unresolved: 0,
			total: units.length,
		} as unknown as PanelList
	}

	async function todayList (): Promise<PanelList | null> {
		try {
			const response = await fetch('/api/panel/list', { credentials: 'same-origin' })
			if (!response.ok) return null
			const data = (await response.json()) as { list?: PanelList }
			return data.list && data.list.entries.length > 0 ? data.list : null
		} catch {
			return null
		}
	}

	const previewHtml = $derived(sheetHtml(sampleList(), JSON.parse(JSON.stringify(template)) as PrintTemplate, 'Vista previa'))

	async function printToday () {
		const list = await todayList()
		if (!list) {
			notify?.('No hay lista publicada para hoy; imprimo la vista previa.', true)
			printSheet(previewHtml)
			return
		}
		const date = new Date().toLocaleDateString('es-CO', { timeZone: 'America/Bogota', weekday: 'long', day: 'numeric', month: 'long' })
		printSheet(sheetHtml(list, JSON.parse(JSON.stringify(template)) as PrintTemplate, date))
	}
</script>

{#if loading}
	<p class="muted">Cargando formato…</p>
{:else}
	<div class="toolbar">
		<span class="muted small">{custom ? 'Formato personalizado' : 'Formato original'}{dirty ? ' · cambios sin guardar' : ''}</span>
		<div class="spacer"></div>
		<button class="btn btn-ghost btn-sm" onclick={discard} disabled={!dirty || busy}>Descartar</button>
		<button class="btn btn-ghost btn-sm" onclick={restore} disabled={busy || !custom}>Restaurar original</button>
		<button class="btn btn-kraft btn-sm" onclick={printToday}>Imprimir / PDF</button>
		<button class="btn btn-primary btn-sm" onclick={save} disabled={!dirty || busy}>Guardar formato</button>
	</div>

	<p class="helper">
		Cada fila de la hoja suma las unidades de cocina que le asignes. Las unidades sin fila salen en «OTROS».
		Para el PDF, elige «Guardar como PDF» en el diálogo de impresión.
	</p>

	{#if missing.length > 0}
		<p class="warn">Unidades que ya no existen en el catálogo: {missing.join(', ')}. Quítalas o fusiónalas.</p>
	{/if}

	<div class="layout">
		<div class="columns">
			{#each ['left', 'right'] as const as side (side)}
				<section class="column">
					<h4>{side === 'left' ? 'Columna izquierda' : 'Columna derecha'}</h4>
					{#each template[side] as block, index (block.key)}
						<div class="block">
							<div class="block-head">
								<input class="input label" bind:value={block.label} aria-label="Nombre de la fila" />
								<div class="tools">
									<button class="icon" title="Subir" onclick={() => moveBlock(side, index, -1)} disabled={index === 0}>↑</button>
									<button class="icon" title="Bajar" onclick={() => moveBlock(side, index, 1)} disabled={index === template[side].length - 1}>↓</button>
									<button class="icon" title="Cambiar de columna" onclick={() => switchSide(side, index)}>{side === 'left' ? '→' : '←'}</button>
									<button class="icon danger" title="Quitar fila" onclick={() => removeBlock(side, index)}>✕</button>
								</div>
							</div>

							{#if block.subRows && block.subRows.length > 0}
								{#each block.subRows as row, rowIndex (row.key)}
									<div class="sub">
										<div class="block-head">
											<input class="input label small-input" bind:value={row.label} aria-label="Nombre de la sub-fila" />
											<button class="icon danger" title="Quitar sub-fila" onclick={() => removeSubRow(block, rowIndex)}>✕</button>
										</div>
										<div class="chips">
											{#each row.unitIds ?? [] as id (id)}
												<span class="chip">{unitName(id)}<button class="x" aria-label="Quitar" onclick={() => removeUnit(row, id)}>×</button></span>
											{/each}
										</div>
										<div class="add">
											<UnitSelect bind:value={() => pick[row.key] ?? '', (value) => (pick[row.key] = value)} />
											<button class="btn btn-ghost btn-sm" onclick={() => addUnit(row, row.key)} disabled={!pick[row.key]}>Añadir</button>
										</div>
									</div>
								{/each}
							{:else}
								<div class="chips">
									{#each block.unitIds ?? [] as id (id)}
										<span class="chip">{unitName(id)}<button class="x" aria-label="Quitar" onclick={() => removeUnit(block, id)}>×</button></span>
									{:else}
										<span class="muted small">Sin unidades: la fila sale vacía.</span>
									{/each}
								</div>
								<div class="add">
									<UnitSelect bind:value={() => pick[block.key] ?? '', (value) => (pick[block.key] = value)} />
									<button class="btn btn-ghost btn-sm" onclick={() => addUnit(block, block.key)} disabled={!pick[block.key]}>Añadir</button>
								</div>
							{/if}
							<button class="link" onclick={() => addSubRow(block)}>+ sub-fila</button>
						</div>
					{/each}
					<button class="btn btn-ghost btn-sm" onclick={() => addBlock(side)}>+ Fila</button>
				</section>
			{/each}
		</div>

		<aside class="side">
			<h4>Sin fila ({unmapped.length})</h4>
			<p class="muted small">Salen en «OTROS» al imprimir.</p>
			<div class="chips">
				{#each unmapped as unit (unit.id)}
					<span class="chip">{unit.name}</span>
				{:else}
					<span class="muted small">Todas las unidades tienen fila.</span>
				{/each}
			</div>
			<h4>Vista previa</h4>
			<iframe class="preview" title="Vista previa de la hoja" srcdoc={previewHtml}></iframe>
		</aside>
	</div>
{/if}

<style>
	.toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		margin-bottom: 8px;
	}
	.spacer {
		flex: 1;
	}
	.helper {
		font-size: 13px;
		color: var(--text-muted);
		margin: 0 0 12px;
	}
	.warn {
		font-size: 13px;
		color: var(--warning, #9c5722);
		margin: 0 0 12px;
	}
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 420px);
		gap: 16px;
		align-items: start;
	}
	.columns {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px;
	}
	.column h4,
	.side h4 {
		margin: 0 0 8px;
		font-size: 13px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--text-muted);
	}
	.side h4 + p {
		margin-top: -4px;
	}
	.block {
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 8px;
		margin-bottom: 8px;
		background: var(--surface);
	}
	.block-head {
		display: flex;
		gap: 6px;
		align-items: center;
	}
	.label {
		flex: 1;
		min-width: 0;
		font-weight: 600;
	}
	.tools {
		display: flex;
		gap: 2px;
	}
	.icon {
		border: 1px solid var(--border);
		background: transparent;
		border-radius: 6px;
		width: 28px;
		height: 28px;
		cursor: pointer;
		color: var(--text);
	}
	.icon:disabled {
		opacity: 0.35;
		cursor: default;
	}
	.icon.danger {
		color: var(--danger);
	}
	.sub {
		margin: 8px 0 0 12px;
		padding-left: 8px;
		border-left: 2px solid var(--border);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
		margin: 6px 0;
	}
	.chip .x {
		border: 0;
		background: transparent;
		cursor: pointer;
		margin-left: 4px;
		color: inherit;
	}
	.add {
		display: flex;
		gap: 6px;
		align-items: center;
	}
	.add :global(.combo) {
		flex: 1;
		min-width: 0;
	}
	.link {
		border: 0;
		background: transparent;
		color: var(--accent);
		cursor: pointer;
		font-size: 12px;
		padding: 4px 0 0;
	}
	.preview {
		width: 100%;
		height: 560px;
		border: 1px solid var(--border);
		border-radius: 8px;
		background: #fff;
	}
	.small {
		font-size: 12px;
	}
	@media (max-width: 1100px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	@media (max-width: 699px) {
		.columns {
			grid-template-columns: minmax(0, 1fr);
		}
		.preview {
			height: 420px;
		}
	}
	.chip {
		max-width: 100%;
		white-space: normal;
		overflow-wrap: anywhere;
	}
	.column,
	.side {
		min-width: 0;
	}
</style>
