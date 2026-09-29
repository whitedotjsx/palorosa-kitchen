<script lang="ts">
	import { slugify } from '@palorosa-kitchen/core'
	import { recompute, editor } from '../lib/store.svelte'

	let newName = $state('')

	function addContainer (): void {
		if (!editor.catalog || !newName.trim()) return
		editor.catalog.containers.push({
			id: slugify(newName),
			name: newName.trim(),
			aliases: [newName.trim()],
			active: true,
		})
		newName = ''
		recompute()
	}
</script>

<div class="toolbar">
	<input placeholder="Nuevo contenedor..." bind:value={newName} />
	<button onclick={addContainer}>Crear</button>
</div>

<table>
	<thead>
		<tr>
			<th>Activo</th>
			<th>Nombre</th>
			<th>Alias</th>
		</tr>
	</thead>
	<tbody>
		{#each editor.catalog?.containers ?? [] as container (container.id)}
			<tr class:inactive={!container.active}>
				<td>
					<input
						type="checkbox"
						checked={container.active}
						onchange={(event) => {
							container.active = event.currentTarget.checked
							recompute()
						}}
					/>
				</td>
				<td>
					<input bind:value={container.name} onchange={recompute} />
					<div class="muted small">{container.id}</div>
				</td>
				<td>
					<input
						value={container.aliases.join(', ')}
						onchange={(event) => {
							container.aliases = event.currentTarget.value
								.split(',')
								.map((alias) => alias.trim())
								.filter(Boolean)
							recompute()
						}}
					/>
				</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	.toolbar {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 0.75rem;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.85rem;
	}
	th,
	td {
		border-bottom: 1px solid var(--line);
		padding: 0.3rem;
	}
	td input {
		width: 100%;
	}
	tr.inactive {
		opacity: 0.55;
	}
	.small {
		font-size: 0.7rem;
	}
</style>
