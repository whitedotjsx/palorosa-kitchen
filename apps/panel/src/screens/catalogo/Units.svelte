<script lang="ts">
	import { slugify, type KitchenUnit } from '@palorosa-kitchen/core'
	import { mergeUnit as merge, recompute, editor, unitName } from '../../lib/catalog.svelte'
	import UnitSelect from './UnitSelect.svelte'

	const CATEGORIES: [KitchenUnit['category'], string][] = [
		['drink', 'Bebidas'],
		['main', 'Comidas'],
		['side', 'Acompañamientos'],
		['dessert', 'Postres'],
		['fruit', 'Frutas'],
		['condiment', 'Condimentos'],
		['other', 'Otros'],
	]

	const MEASURES: [KitchenUnit['measure'], string][] = [
		['unit', 'Unidad'],
		['portion', 'Porción'],
		['glass', 'Vaso'],
		['bottle', 'Botella'],
		['slice', 'Tajada'],
		['spoon', 'Cucharada'],
		['gram', 'Gramo'],
	]

	const categoryLabel = Object.fromEntries(CATEGORIES) as Record<string, string>
	const measureLabel = Object.fromEntries(MEASURES) as Record<string, string>

	let query = $state('')
	let filter = $state<'all' | 'active' | 'kitchen' | 'inactive'>('active')
	let category = $state<'all' | KitchenUnit['category']>('all')
	let selectedId = $state('')
	let newName = $state('')
	let mergeTarget = $state('')

	const units = $derived(
		[...(editor.catalog?.units ?? [])]
			.filter(
				(unit) =>
					(filter === 'all' ||
						(filter === 'active' && unit.active) ||
						(filter === 'kitchen' && unit.active && unit.isKitchen) ||
						(filter === 'inactive' && !unit.active)) &&
					(category === 'all' || unit.category === category) &&
					(unit.name.toLowerCase().includes(query.toLowerCase()) ||
						unit.id.includes(query.toLowerCase()))
			)
			.sort((a, b) => a.name.localeCompare(b.name))
	)

	const unit = $derived(editor.catalog?.units.find((item) => item.id === selectedId))

	function select (id: string): void {
		selectedId = id
		mergeTarget = ''
	}

	function addUnit (): void {
		const name = newName.trim()
		if (!editor.catalog || !name) return
		const id = slugify(name)
		if (editor.catalog.units.some((item) => item.id === id)) {
			select(id)
			newName = ''
			return
		}
		editor.catalog.units.push({
			id,
			name,
			category: 'other',
			measure: 'unit',
			aliases: [name],
			isKitchen: true,
			active: true,
		})
		newName = ''
		query = ''
		recompute()
		select(id)
	}

	function doMerge (): void {
		if (!unit || !mergeTarget || mergeTarget === unit.id) return
		if (!window.confirm(`¿Fusionar «${unit.name}» en «${unitName(mergeTarget)}»? La primera queda inactiva y sus recetas pasan a la segunda.`)) return
		merge(unit.id, mergeTarget)
		select(mergeTarget)
	}

	function splitAliases (value: string): string[] {
		return value
			.split(',')
			.map((alias) => alias.trim())
			.filter(Boolean)
	}
</script>

<div class="toolbar">
	<div class="seg">
		<button type="button" class:active={filter === 'active'} onclick={() => (filter = 'active')}>Activas</button>
		<button type="button" class:active={filter === 'kitchen'} onclick={() => (filter = 'kitchen')}>Cocina</button>
		<button type="button" class:active={filter === 'inactive'} onclick={() => (filter = 'inactive')}>Inactivas</button>
		<button type="button" class:active={filter === 'all'} onclick={() => (filter = 'all')}>Todas</button>
	</div>
	<div class="select" style="flex:0 0 auto;min-width:200px">
		<select bind:value={category} aria-label="Categoría">
			<option value="all">Todas las categorías</option>
			{#each CATEGORIES as [value, label] (value)}<option {value}>{label}</option>{/each}
		</select>
	</div>
	<div class="search">
		<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
		<input class="input" bind:value={query} placeholder="Buscar unidad…" />
	</div>
</div>

<div class="with-drawer">
	<div class="panel">
		<table class="table">
			<thead>
				<tr>
					<th>Unidad</th>
					<th>Categoría</th>
					<th>Medida</th>
					<th>Estado</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each units as item (item.id)}
					<tr class="row-click" class:selected={item.id === selectedId} onclick={() => select(item.id)}>
						<td class="cell-acct" data-label="Unidad">
							<b class="unit-name">{item.name}</b>
							<span>{item.aliases.length > 1 ? `${item.aliases.length} alias` : item.id}</span>
						</td>
						<td data-label="Categoría"><span class="chip">{categoryLabel[item.category] ?? item.category}</span></td>
						<td data-label="Medida">{measureLabel[item.measure] ?? item.measure}</td>
						<td data-label="Estado">
							{#if editor.merges[item.id]}
								<span class="stamp jam">Fusionada</span>
							{:else if !item.active}
								<span class="stamp neutral">Inactiva</span>
							{:else if !item.isKitchen}
								<span class="stamp honey">No cocina</span>
							{:else}
								<span class="stamp herb">Cocina</span>
							{/if}
						</td>
						<td class="acc"><button class="btn btn-kraft btn-sm" aria-label="Editar">✎</button></td>
					</tr>
				{/each}
				{#if units.length === 0}
					<tr><td colspan="5" class="muted">Sin unidades con ese filtro.</td></tr>
				{/if}
			</tbody>
		</table>
	</div>

	<aside class="recipe editor-side" style="margin:0">
		{#if unit}
			<div class="recipe-head">
				<h3>{unit.name}</h3>
				<button class="btn btn-ghost btn-sm" style="margin-left:auto" onclick={() => select('')}>Cerrar</button>
			</div>
			<p class="helper" style="margin:-6px 0 14px">id {unit.id}</p>

			<div class="field">
				<label class="label" for="u-name">Nombre</label>
				<input class="input" id="u-name" bind:value={unit.name} onchange={recompute} />
			</div>

			<div class="field-row">
				<div class="field">
					<label class="label" for="u-cat">Categoría</label>
					<div class="select">
						<select id="u-cat" bind:value={unit.category} onchange={recompute}>
							{#each CATEGORIES as [value, label] (value)}<option {value}>{label}</option>{/each}
						</select>
					</div>
				</div>
				<div class="field">
					<label class="label" for="u-measure">Medida</label>
					<div class="select">
						<select id="u-measure" bind:value={unit.measure} onchange={recompute}>
							{#each MEASURES as [value, label] (value)}<option {value}>{label}</option>{/each}
						</select>
					</div>
				</div>
			</div>

			<div class="field">
				<label class="label" for="u-aliases">Alias</label>
				<textarea
					class="input"
					id="u-aliases"
					rows="2"
					value={unit.aliases.join(', ')}
					onchange={(event) => {
						unit.aliases = splitAliases(event.currentTarget.value)
						recompute()
					}}
				></textarea>
				<p class="helper">Separados por coma. Así puede aparecer en los pedidos.</p>
			</div>

			<div class="field">
				<label class="label" for="u-desc">Descripción</label>
				<textarea
					class="input"
					id="u-desc"
					rows="2"
					value={unit.description ?? ''}
					onchange={(event) => {
						unit.description = event.currentTarget.value
						recompute()
					}}
				></textarea>
			</div>

			<div class="field">
				<label class="label" for="u-note">Nota para cocina</label>
				<input
					class="input"
					id="u-note"
					value={unit.note ?? ''}
					onchange={(event) => {
						unit.note = event.currentTarget.value
						recompute()
					}}
				/>
			</div>

			<div class="checks field">
				<label class="check"><input type="checkbox" bind:checked={unit.active} onchange={recompute} /> Activa</label>
				<label class="check"><input type="checkbox" bind:checked={unit.isKitchen} onchange={recompute} /> Va a cocina</label>
			</div>

			<div class="drawer-foot" style="flex-direction:column;align-items:stretch">
				{#if editor.merges[unit.id]}
					<p class="helper" style="margin:0">Fusionada en <b>{unitName(editor.merges[unit.id] ?? '')}</b>.</p>
				{:else}
					<span class="label" style="margin:0">Fusionar en otra unidad</span>
					<div class="add-row" style="margin:0">
						<UnitSelect bind:value={mergeTarget} placeholder="Elegir unidad destino…" />
						<button class="btn btn-danger btn-sm" disabled={!mergeTarget || mergeTarget === unit.id} onclick={doMerge}>Fusionar</button>
					</div>
				{/if}
			</div>
		{:else}
			<div class="recipe-head"><h3>Nueva unidad</h3></div>
			<p class="helper" style="margin:-4px 0 14px">Elige una unidad de la tabla para editarla, o crea una nueva.</p>
			<div class="field">
				<label class="label" for="u-new">Nombre</label>
				<input
					class="input"
					id="u-new"
					bind:value={newName}
					placeholder="p. ej. Jugo de mango"
					onkeydown={(event) => event.key === 'Enter' && addUnit()}
				/>
			</div>
			<div class="drawer-foot">
				<button class="btn btn-primary btn-sm" disabled={!newName.trim()} onclick={addUnit}>Crear unidad</button>
			</div>
		{/if}
	</aside>
</div>
