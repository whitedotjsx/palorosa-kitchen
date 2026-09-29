<script lang="ts">
	import { slugify, type KitchenUnit } from '@palorosa-kitchen/core'
	import { mergeUnit as merge, recompute, editor } from '../lib/store.svelte'

	let query = $state('')
	let showInactive = $state(true)
	let newName = $state('')
	let mergeTarget = $state('')

	const CATEGORIES: [string, string][] = [
		['drink', 'Bebidas'],
		['main', 'Comidas'],
		['side', 'Acompañamientos'],
		['dessert', 'Postres'],
		['fruit', 'Frutas'],
		['condiment', 'Condimentos'],
		['other', 'Otros'],
	]

	const MEASURES: [string, string][] = [
		['unit', 'Unidad'],
		['portion', 'Porción'],
		['glass', 'Vaso'],
		['bottle', 'Botella'],
		['slice', 'Tajada'],
		['spoon', 'Cucharada'],
		['gram', 'Gramo'],
	]

	const units = $derived(
		[...(editor.catalog?.units ?? [])]
			.filter(
				(unit) =>
					(showInactive || unit.active) &&
					unit.name.toLowerCase().includes(query.toLowerCase())
			)
			.sort((a, b) => a.name.localeCompare(b.name))
	)

	function addUnit (): void {
		if (!editor.catalog || !newName.trim()) return
		editor.catalog.units.push({
			id: slugify(newName),
			name: newName.trim(),
			category: 'other',
			measure: 'unit',
			aliases: [newName.trim()],
			isKitchen: true,
			active: true,
		})
		newName = ''
		recompute()
	}

	function mergeUnit (unit: KitchenUnit): void {
		if (!mergeTarget) return
		merge(unit.id, mergeTarget)
		mergeTarget = ''
	}

	function setAliases (unit: KitchenUnit, value: string): void {
		unit.aliases = value
			.split(',')
			.map((alias) => alias.trim())
			.filter(Boolean)
		recompute()
	}
</script>

<div class="toolbar">
	<input placeholder="Buscar unidad..." bind:value={query} />
	<label class="check"><input type="checkbox" bind:checked={showInactive} /> Ver inactivas</label>
	<input placeholder="Nueva unidad..." bind:value={newName} />
	<button onclick={addUnit}>Crear</button>
</div>

<table>
	<thead>
		<tr>
			<th>Activa</th>
			<th>Cocina</th>
			<th>Nombre</th>
			<th>Descripción</th>
			<th>Nota</th>
			<th>Categoría</th>
			<th>Medida</th>
			<th>Alias</th>
			<th>Fusionar en</th>
		</tr>
	</thead>
	<tbody>
		{#each units as unit (unit.id)}
			<tr class:inactive={!unit.active}>
				<td>
					<input
						type="checkbox"
						checked={unit.active}
						onchange={(event) => {
							unit.active = event.currentTarget.checked
							recompute()
						}}
					/>
				</td>
				<td>
					<input
						type="checkbox"
						checked={unit.isKitchen}
						onchange={(event) => {
							unit.isKitchen = event.currentTarget.checked
							recompute()
						}}
					/>
				</td>
				<td>
					<input bind:value={unit.name} onchange={recompute} />
					<div class="muted small">{unit.id}</div>
				</td>
				<td>
					<textarea
						value={unit.description ?? ''}
						onchange={(event) => {
							unit.description = event.currentTarget.value
							recompute()
						}}
					></textarea>
				</td>
				<td>
					<input
						value={unit.note ?? ''}
						onchange={(event) => {
							unit.note = event.currentTarget.value
							recompute()
						}}
					/>
				</td>
				<td>
					<select
						value={unit.category}
						onchange={(event) => {
							unit.category = event.currentTarget.value as KitchenUnit['category']
							recompute()
						}}
					>
						{#each CATEGORIES as [value, label] (value)}
							<option {value}>{label}</option>
						{/each}
					</select>
				</td>
				<td>
					<select
						value={unit.measure}
						onchange={(event) => {
							unit.measure = event.currentTarget.value as KitchenUnit['measure']
							recompute()
						}}
					>
						{#each MEASURES as [value, label] (value)}
							<option {value}>{label}</option>
						{/each}
					</select>
				</td>
				<td>
					<input
						value={unit.aliases.join(', ')}
						onchange={(event) => setAliases(unit, event.currentTarget.value)}
					/>
				</td>
				<td>
					{#if editor.merges[unit.id]}
						<span class="muted small">→ {editor.merges[unit.id]}</span>
					{:else}
						<select bind:value={mergeTarget}>
							<option value="">(elegir)</option>
							{#each units as candidate (candidate.id)}
								{#if candidate.id !== unit.id}
									<option value={candidate.id}>{candidate.name}</option>
								{/if}
							{/each}
						</select>
						<button onclick={() => mergeUnit(unit)}>Fusionar</button>
					{/if}
				</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.85rem;
	}
	th,
	td {
		border-bottom: 1px solid var(--line);
		padding: 0.3rem;
		vertical-align: top;
	}
	td input,
	td select,
	td textarea {
		width: 100%;
	}
	td textarea {
		min-height: 3.5rem;
	}
	tr.inactive {
		opacity: 0.55;
	}
	.toolbar {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		margin-bottom: 0.75rem;
		flex-wrap: wrap;
	}
	.check {
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}
	.small {
		font-size: 0.7rem;
	}
</style>
