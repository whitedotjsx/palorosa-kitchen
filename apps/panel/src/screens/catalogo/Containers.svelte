<script lang="ts">
	import { slugify } from '@palorosa-kitchen/core'
	import { recompute, editor } from '../../lib/catalog.svelte'

	let query = $state('')
	let selectedId = $state('')
	let newName = $state('')

	const containers = $derived(
		[...(editor.catalog?.containers ?? [])]
			.filter((item) => item.name.toLowerCase().includes(query.toLowerCase()))
			.sort((a, b) => Number(b.active) - Number(a.active) || a.name.localeCompare(b.name))
	)

	const container = $derived(editor.catalog?.containers.find((item) => item.id === selectedId))

	function addContainer (): void {
		const name = newName.trim()
		if (!editor.catalog || !name) return
		const id = slugify(name)
		if (!editor.catalog.containers.some((item) => item.id === id)) {
			editor.catalog.containers.push({ id, name, aliases: [name], active: true })
			recompute()
		}
		newName = ''
		selectedId = id
	}
</script>

<div class="toolbar">
	<span class="helper" style="margin:0">Cajas, canastas y bolsas en las que se entrega el desayuno.</span>
	<div class="search">
		<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
		<input class="input" bind:value={query} placeholder="Buscar contenedor…" />
	</div>
</div>

<div class="with-drawer">
	<div class="panel">
		<table class="table">
			<thead>
				<tr>
					<th>Contenedor</th>
					<th>Alias</th>
					<th>Estado</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each containers as item (item.id)}
					<tr class="row-click" class:selected={item.id === selectedId} onclick={() => (selectedId = item.id)}>
						<td class="cell-acct" data-label="Contenedor"><b class="unit-name">{item.name}</b></td>
						<td data-label="Alias" class="num">{item.aliases.length}</td>
						<td data-label="Estado">
							<span class="stamp" class:herb={item.active} class:neutral={!item.active}>{item.active ? 'Activo' : 'Inactivo'}</span>
						</td>
						<td class="acc"><button class="btn btn-kraft btn-sm" aria-label="Editar">✎</button></td>
					</tr>
				{/each}
				{#if containers.length === 0}
					<tr><td colspan="4" class="muted">Sin contenedores.</td></tr>
				{/if}
			</tbody>
		</table>
	</div>

	<aside class="recipe editor-side" style="margin:0">
		{#if container}
			<div class="recipe-head">
				<h3>Editar contenedor</h3>
				<button class="btn btn-ghost btn-sm" style="margin-left:auto" onclick={() => (selectedId = '')}>Cerrar</button>
			</div>
			<p class="helper" style="margin:-6px 0 14px">id {container.id}</p>
			<div class="field">
				<label class="label" for="c-name">Nombre</label>
				<textarea class="input" id="c-name" rows="2" bind:value={container.name} onchange={recompute}></textarea>
			</div>
			<div class="field">
				<label class="label" for="c-aliases">Alias</label>
				<textarea
					class="input"
					id="c-aliases"
					rows="3"
					value={container.aliases.join(', ')}
					onchange={(event) => {
						container.aliases = event.currentTarget.value
							.split(',')
							.map((alias) => alias.trim())
							.filter(Boolean)
						recompute()
					}}
				></textarea>
				<p class="helper">Separados por coma.</p>
			</div>
			<label class="check"><input type="checkbox" bind:checked={container.active} onchange={recompute} /> Activo</label>
		{:else}
			<div class="recipe-head"><h3>Nuevo contenedor</h3></div>
			<p class="helper" style="margin:-4px 0 14px">Elige uno de la tabla para editarlo, o crea uno nuevo.</p>
			<div class="field">
				<label class="label" for="c-new">Nombre</label>
				<input
					class="input"
					id="c-new"
					bind:value={newName}
					placeholder="p. ej. Caja kraft mediana"
					onkeydown={(event) => event.key === 'Enter' && addContainer()}
				/>
			</div>
			<div class="drawer-foot">
				<button class="btn btn-primary btn-sm" disabled={!newName.trim()} onclick={addContainer}>Crear contenedor</button>
			</div>
		{/if}
	</aside>
</div>
