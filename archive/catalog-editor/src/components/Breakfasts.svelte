<script lang="ts">
	import { recompute, editor } from '../lib/store.svelte'
	import RecipeEditor from './RecipeEditor.svelte'

	let query = $state('')
	let selected = $state('')

	const breakfasts = $derived(
		(editor.catalog?.products ?? [])
			.filter(
				(product) =>
					product.category === 'breakfast' &&
					product.name.toLowerCase().includes(query.toLowerCase())
			)
			.sort((a, b) => a.name.localeCompare(b.name))
	)

	const product = $derived(breakfasts.find((item) => item.id === selected) ?? breakfasts[0])
</script>

<div class="split">
	<aside>
		<input placeholder="Buscar desayuno..." bind:value={query} />
		<ul>
			{#each breakfasts as item (item.id)}
				<li>
					<button class:active={item.id === product?.id} onclick={() => (selected = item.id)}>
						{item.name}{item.active ? '' : ' (inactivo)'}
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
				<p class="muted">id: {product.id} · WP: {product.wpProductId ?? 'sin id'} · {product.category}</p>
			</div>

			<h3>Receta</h3>
			<RecipeEditor productId={product.id} />
		{:else}
			<p>Sin desayunos.</p>
		{/if}
	</section>
</div>

<style>
	.split {
		display: grid;
		grid-template-columns: 16rem 1fr;
		gap: 1rem;
	}
	aside ul {
		list-style: none;
		padding: 0;
		margin: 0.5rem 0 0;
		max-height: 70vh;
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
		min-height: 4rem;
	}
	.check {
		flex-direction: row !important;
		align-items: center;
	}
</style>
