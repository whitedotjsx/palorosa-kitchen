<script lang="ts">
	import { recompute, editor } from '../lib/store.svelte'
	import RecipeEditor from './RecipeEditor.svelte'

	let query = $state('')
	let category = $state('all')
	let kitchen = $state('all')
	let selected = $state('')

	const products = $derived(
		(editor.catalog?.products ?? [])
			.filter(
				(product) =>
					(category === 'all' || product.category === category) &&
					(kitchen === 'all' ||
						(kitchen === 'yes' && product.isKitchen) ||
						(kitchen === 'no' && !product.isKitchen)) &&
					product.name.toLowerCase().includes(query.toLowerCase())
			)
			.sort((a, b) => a.name.localeCompare(b.name))
	)

	const product = $derived(products.find((item) => item.id === selected) ?? products[0])

	const categories = $derived([
		...new Set((editor.catalog?.products ?? []).map((item) => item.category)),
	])
</script>

<div class="split">
	<aside>
		<input placeholder="Buscar producto..." bind:value={query} />
		<select bind:value={category}>
			<option value="all">Todas las categorías</option>
			{#each categories as item (item)}
				<option value={item}>{item}</option>
			{/each}
		</select>
		<select bind:value={kitchen}>
			<option value="all">Cocina y no cocina</option>
			<option value="yes">Solo cocina</option>
			<option value="no">Solo no cocina</option>
		</select>
		<ul>
			{#each products as item (item.id)}
				<li>
					<button
						class:active={item.id === product?.id}
						class:non-kitchen={!item.isKitchen}
						onclick={() => (selected = item.id)}
					>
						{item.name}{item.active ? '' : ' (inactivo)'}{item.isKitchen ? '' : ' · no cocina'}
					</button>
				</li>
			{/each}
		</ul>
	</aside>

	<section>
		{#if product}
			<h2>{product.name}</h2>
			<div class="fields">
				<label>Nombre <input bind:value={product.name} onchange={recompute} /></label>
				<label class="check">
					<input type="checkbox" bind:checked={product.active} onchange={recompute} /> Activo
				</label>
				<label class="check">
					<input type="checkbox" bind:checked={product.isKitchen} onchange={recompute} /> Va a cocina
				</label>
				<label>
					Alias
					<input
						value={product.aliases.join(', ')}
						onchange={(event) => {
							product.aliases = event.currentTarget.value
								.split(',')
								.map((alias) => alias.trim())
								.filter(Boolean)
							recompute()
						}}
					/>
				</label>
				<label class="full">
					Descripción
					<textarea
						value={product.description ?? ''}
						onchange={(event) => {
							product.description = event.currentTarget.value
							recompute()
						}}
					></textarea>
				</label>
				<p class="muted">
					id: {product.id} · categoría: {product.category} · WP: {product.wpProductId ?? 'sin id'} · SKU: {product.sku ?? 'sin sku'} · precio: {product.priceCop ?? 'sin precio'} · cocina: {product.isKitchen ? 'sí' : 'no'}
				</p>
			</div>

			{#if product.category === 'breakfast' || product.category === 'add_on'}
				<h3>Receta</h3>
				<RecipeEditor productId={product.id} />
			{/if}
		{:else}
			<p>Sin productos.</p>
		{/if}
	</section>
</div>

<style>
	.split {
		display: grid;
		grid-template-columns: 18rem 1fr;
		gap: 1rem;
	}
	aside {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	aside ul {
		list-style: none;
		padding: 0;
		margin: 0;
		max-height: 65vh;
		overflow: auto;
	}
	aside button {
		width: 100%;
		text-align: left;
	}
	aside button.active {
		font-weight: 600;
		background: var(--accent-soft);
	}
	aside button.non-kitchen {
		opacity: 0.65;
	}
	.fields {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		align-items: center;
	}
	.fields label {
		display: flex;
		flex-direction: column;
		font-size: 0.8rem;
		gap: 0.2rem;
	}
	.fields .full {
		flex: 1 0 100%;
	}
	textarea {
		min-height: 5rem;
	}
	.check {
		flex-direction: row !important;
		align-items: center;
	}
</style>
