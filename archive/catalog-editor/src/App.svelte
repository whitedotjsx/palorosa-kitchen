<script lang="ts">
	import { onMount } from 'svelte'
	import Breakfasts from './components/Breakfasts.svelte'
	import Containers from './components/Containers.svelte'
	import Diff from './components/Diff.svelte'
	import Duplicates from './components/Duplicates.svelte'
	import Products from './components/Products.svelte'
	import Units from './components/Units.svelte'
	import { load, save, editor } from './lib/store.svelte'

	type Tab = 'desayunos' | 'unidades' | 'duplicados' | 'productos' | 'contenedores' | 'cambios'

	const tabs: [Tab, string][] = [
		['desayunos', 'Desayunos'],
		['unidades', 'Unidades'],
		['duplicados', 'Duplicados'],
		['productos', 'Productos'],
		['contenedores', 'Contenedores'],
		['cambios', 'Cambios'],
	]

	let tab = $state<Tab>('desayunos')

	onMount(load)

	const stats = $derived({
		units: editor.catalog?.units.length ?? 0,
		products: editor.catalog?.products.length ?? 0,
		recipes: editor.catalog?.recipes.length ?? 0,
	})
</script>

<div class="app">
	<header>
		<h1>Catálogo Palorosa</h1>
		<span class="muted">
			{stats.units} unidades · {stats.products} productos · {stats.recipes} recetas
		</span>
		<button class="primary" onclick={save} disabled={editor.saving}>
			{editor.saving ? 'Guardando...' : 'Guardar cambios'}
		</button>
	</header>

	<nav>
		{#each tabs as [value, label] (value)}
			<button class:active={tab === value} onclick={() => (tab = value)}>{label}</button>
		{/each}
	</nav>

	{#if editor.error}
		<p class="error">{editor.error}</p>
	{/if}

	{#if editor.loading}
		<p>Cargando catálogo...</p>
	{:else}
		{#if tab === 'desayunos'}<Breakfasts />{/if}
		{#if tab === 'unidades'}<Units />{/if}
		{#if tab === 'duplicados'}<Duplicates />{/if}
		{#if tab === 'productos'}<Products />{/if}
		{#if tab === 'contenedores'}<Containers />{/if}
		{#if tab === 'cambios'}<Diff />{/if}
	{/if}

	{#if editor.message}
		<pre class="message">{editor.message}</pre>
	{/if}
</div>

<style>
	:global(:root) {
		--bg: #faf7f2;
		--panel: #f1ece3;
		--line: #d9d2c5;
		--text: #2c2620;
		--muted: #7a7267;
		--accent: #7a5c3e;
		--accent-soft: #efe3d3;
	}
	:global(body) {
		margin: 0;
		background: var(--bg);
		color: var(--text);
		font-family: system-ui, 'Segoe UI', sans-serif;
	}
	:global(input),
	:global(select),
	:global(textarea),
	:global(button) {
		font: inherit;
		padding: 0.3rem 0.45rem;
		border: 1px solid var(--line);
		border-radius: 5px;
		background: #fff;
		color: inherit;
	}
	:global(button) {
		cursor: pointer;
	}
	:global(button.primary) {
		background: var(--accent);
		color: #fff;
		border-color: var(--accent);
	}
	.app {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1rem;
	}
	header {
		display: flex;
		gap: 0.75rem;
		align-items: center;
		flex-wrap: wrap;
	}
	h1 {
		font-size: 1.25rem;
		margin: 0;
	}
	nav {
		display: flex;
		gap: 0.35rem;
		margin: 0.75rem 0;
		border-bottom: 1px solid var(--line);
		padding-bottom: 0.4rem;
	}
	nav button {
		background: transparent;
		border: none;
	}
	nav button.active {
		background: var(--accent-soft);
		font-weight: 600;
	}
	.muted {
		color: var(--muted);
		font-size: 0.85rem;
	}
	.error {
		color: #a33;
	}
	.message {
		background: var(--panel);
		padding: 0.6rem;
		border-radius: 6px;
		font-size: 0.8rem;
		white-space: pre-wrap;
	}
</style>
