<script lang="ts">
	import { onMount } from 'svelte'
	import { discard, download, editor, load, save } from '../lib/catalog.svelte'
	import Breakfasts from './catalogo/Breakfasts.svelte'
	import Containers from './catalogo/Containers.svelte'
	import Duplicates from './catalogo/Duplicates.svelte'
	import PrintFormat from './catalogo/PrintFormat.svelte'
	import Products from './catalogo/Products.svelte'
	import Units from './catalogo/Units.svelte'

	interface Props {
		notify?: (message: string, error?: boolean) => void
	}
	let { notify }: Props = $props()

	type Tab = 'desayunos' | 'unidades' | 'duplicados' | 'productos' | 'contenedores' | 'formato'

	const tabs: [Tab, string][] = [
		['desayunos', 'Desayunos'],
		['productos', 'Productos'],
		['unidades', 'Unidades'],
		['duplicados', 'Duplicados'],
		['contenedores', 'Contenedores'],
		['formato', 'Formato PDF'],
	]

	let tab = $state<Tab>('desayunos')

	onMount(() => {
		if (!editor.catalog) void load()
	})

	const stats = $derived({
		units: editor.catalog?.units.length ?? 0,
		products: editor.catalog?.products.length ?? 0,
		recipes: editor.catalog?.recipes.length ?? 0,
	})

	async function onSave () {
		try {
			const result = await save()
			notify?.(`Catálogo guardado: ${result.products} productos, ${result.recipes} recetas. La cocina lo usa desde ya.`)
		} catch (error) {
			notify?.((error as Error).message, true)
		}
	}

	function onDiscard () {
		if (window.confirm('¿Descartar los cambios sin guardar?')) discard()
	}

	function onBackup () {
		if (editor.catalog) download(`catalogo-${new Date().toISOString().slice(0, 10)}.json`, $state.snapshot(editor.catalog))
	}

	// Leaving the page with unsaved catalog edits asks first.
	function beforeUnload (event: BeforeUnloadEvent) {
		if (editor.dirty) event.preventDefault()
	}
</script>

<svelte:window onbeforeunload={beforeUnload} />

<div class="screen">
	<div class="cat-head">
		<span class="muted small">
			{stats.units} unidades · {stats.products} productos · {stats.recipes} recetas
			{#if editor.dirty}<b class="dirty"> · cambios sin guardar</b>{/if}
		</span>
		<div class="spacer"></div>
		{#if tab !== 'formato'}
			<button class="btn btn-ghost btn-sm" onclick={onBackup} disabled={!editor.catalog}>Descargar copia</button>
			<button class="btn btn-ghost btn-sm" onclick={onDiscard} disabled={!editor.dirty || editor.saving}>Descartar</button>
			<button class="btn btn-primary btn-sm" onclick={onSave} disabled={!editor.dirty || editor.saving}>
				{editor.saving ? 'Guardando…' : 'Guardar catálogo'}
			</button>
		{/if}
	</div>

	<nav class="tabs" aria-label="Secciones del catálogo">
		{#each tabs as [value, label] (value)}
			<button class:active={tab === value} aria-pressed={tab === value} onclick={() => (tab = value)}>{label}</button>
		{/each}
	</nav>

	{#if editor.error}
		<p class="error">{editor.error} <button class="btn btn-ghost btn-sm" onclick={load}>Reintentar</button></p>
	{/if}

	{#if editor.problems.length > 0}
		<div class="error">
			<b>No se guardó, el catálogo tiene problemas:</b>
			<ul>
				{#each editor.problems as problem, index (index)}<li>{problem}</li>{/each}
			</ul>
		</div>
	{/if}

	{#if editor.loading}
		<p class="muted">Cargando catálogo…</p>
	{:else if editor.catalog}
		{#if tab === 'desayunos'}<Breakfasts />{/if}
		{#if tab === 'unidades'}<Units />{/if}
		{#if tab === 'duplicados'}<Duplicates />{/if}
		{#if tab === 'productos'}<Products />{/if}
		{#if tab === 'contenedores'}<Containers />{/if}
		{#if tab === 'formato'}<PrintFormat {notify} />{/if}
	{/if}
</div>

<style>
	.cat-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		margin-bottom: 10px;
	}
	.spacer {
		flex: 1;
	}
	.small {
		font-size: 13px;
	}
	.dirty {
		color: var(--warning, #9c5722);
		font-weight: 600;
	}
	.tabs {
		display: flex;
		gap: 4px;
		overflow-x: auto;
		border-bottom: 1px solid var(--border);
		margin-bottom: 14px;
		scrollbar-width: none;
	}
	.tabs button {
		flex: none;
		border: 0;
		background: transparent;
		padding: 8px 12px;
		font: inherit;
		font-size: 14px;
		color: var(--text-muted);
		border-bottom: 2px solid transparent;
		cursor: pointer;
	}
	.tabs button.active {
		color: var(--text);
		border-bottom-color: var(--accent);
		font-weight: 600;
	}
	.error {
		color: var(--danger);
		font-size: 13px;
		margin: 0 0 12px;
	}
	.error ul {
		margin: 4px 0 0;
		padding-left: 18px;
	}
</style>
