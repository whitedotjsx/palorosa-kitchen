<script lang="ts">
	import type { ProductCategory } from '@palorosa-kitchen/core'
	import { recompute, editor } from '../../lib/catalog.svelte'
	import RecipeEditor from './RecipeEditor.svelte'

	let { breakfastsOnly = false }: { breakfastsOnly?: boolean } = $props()

	const CATEGORY_LABELS: Record<ProductCategory, string> = {
		breakfast: 'Desayuno',
		add_on: 'Adicional',
		flower: 'Flores',
		stuffed_animal: 'Peluche',
		accessory: 'Accesorio',
		gift_box: 'Caja regalo',
		bag: 'Bolsa',
	}

	let query = $state('')
	let category = $state<'all' | ProductCategory>('all')
	let kitchen = $state<'all' | 'yes' | 'no'>('all')
	let selected = $state('')

	const products = $derived(
		(editor.catalog?.products ?? [])
			.filter(
				(product) =>
					(breakfastsOnly
						? product.category === 'breakfast'
						: category === 'all' || product.category === category) &&
					(kitchen === 'all' ||
						(kitchen === 'yes' && product.isKitchen) ||
						(kitchen === 'no' && !product.isKitchen)) &&
					product.name.toLowerCase().includes(query.toLowerCase())
			)
			.sort((a, b) => Number(b.active) - Number(a.active) || a.name.localeCompare(b.name))
	)

	const product = $derived(products.find((item) => item.id === selected) ?? products[0])

	const categories = $derived(
		[...new Set((editor.catalog?.products ?? []).map((item) => item.category))].sort()
	)

	const recipeSize = (id: string): number =>
		editor.catalog?.recipes.find((recipe) => recipe.productId === id)?.components.length ?? 0

	function splitAliases (value: string): string[] {
		return value
			.split(',')
			.map((alias) => alias.trim())
			.filter(Boolean)
	}
</script>

<div class="cat-split">
	<div class="panel cat-list">
		<div class="cat-list-head">
			<div class="search" style="margin:0;width:100%">
				<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
				<input class="input" bind:value={query} placeholder={breakfastsOnly ? 'Buscar desayuno…' : 'Buscar producto…'} />
			</div>
			{#if !breakfastsOnly}
				<div class="field-row" style="gap:8px">
					<div class="select">
						<select bind:value={category} aria-label="Categoría">
							<option value="all">Todas</option>
							{#each categories as item (item)}
								<option value={item}>{CATEGORY_LABELS[item] ?? item}</option>
							{/each}
						</select>
					</div>
					<div class="select">
						<select bind:value={kitchen} aria-label="Cocina">
							<option value="all">Cocina y no</option>
							<option value="yes">Solo cocina</option>
							<option value="no">No cocina</option>
						</select>
					</div>
				</div>
			{/if}
			<span class="helper" style="margin:0">{products.length} {breakfastsOnly ? 'desayunos' : 'productos'}</span>
		</div>

		<ul class="cat-rows">
			{#each products as item (item.id)}
				<li>
					<button class:active={item.id === product?.id} class:off={!item.active} onclick={() => (selected = item.id)}>
						<span class="cat-row-main">
							<b>{item.name}</b>
							<span>
								{#if !breakfastsOnly}{CATEGORY_LABELS[item.category] ?? item.category} · {/if}{recipeSize(item.id)} componentes
							</span>
						</span>
						{#if !item.active}<span class="stamp neutral">Inactivo</span>{:else if !item.isKitchen}<span class="stamp neutral">No cocina</span>{/if}
					</button>
				</li>
			{/each}
			{#if products.length === 0}
				<li class="muted" style="padding:14px">Sin resultados.</li>
			{/if}
		</ul>
	</div>

	{#if product}
		<div class="cat-detail">
			<section class="recipe">
				<div class="recipe-head">
					<h3>{product.name}</h3>
					<span class="stamp" class:herb={product.active} class:neutral={!product.active}>
						{product.active ? 'Activo' : 'Inactivo'}
					</span>
				</div>

				<div class="chips" style="margin-bottom:14px">
					<span class="chip">{CATEGORY_LABELS[product.category] ?? product.category}</span>
					<span class="chip mono">id {product.id}</span>
					<span class="chip mono">WP {product.wpProductId ?? '—'}</span>
					{#if product.sku}<span class="chip mono">SKU {product.sku}</span>{/if}
					{#if product.priceCop}<span class="chip mono">$ {product.priceCop.toLocaleString('es-CO')}</span>{/if}
				</div>

				<div class="field">
					<label class="label" for="p-name">Nombre</label>
					<input class="input" id="p-name" bind:value={product.name} onchange={recompute} />
				</div>

				<div class="field">
					<label class="label" for="p-aliases">Alias</label>
					<input
						class="input"
						id="p-aliases"
						value={product.aliases.join(', ')}
						placeholder="Otros nombres con los que llega en el pedido, separados por coma"
						onchange={(event) => {
							product.aliases = splitAliases(event.currentTarget.value)
							recompute()
						}}
					/>
				</div>

				<div class="field">
					<label class="label" for="p-desc">Descripción</label>
					<textarea
						class="input"
						id="p-desc"
						rows="3"
						value={product.description ?? ''}
						onchange={(event) => {
							product.description = event.currentTarget.value
							recompute()
						}}
					></textarea>
				</div>

				<div class="checks" style="margin-top:4px">
					<label class="check"><input type="checkbox" bind:checked={product.active} onchange={recompute} /> Activo</label>
					<label class="check"><input type="checkbox" bind:checked={product.isKitchen} onchange={recompute} /> Va a cocina</label>
				</div>
			</section>

			{#if product.category === 'breakfast' || product.category === 'add_on'}
				<section class="recipe">
					<div class="recipe-head">
						<h3>Receta</h3>
						<span class="pin">{recipeSize(product.id)} componentes</span>
					</div>
					<RecipeEditor productId={product.id} />
				</section>
			{/if}
		</div>
	{:else}
		<p class="muted">Sin {breakfastsOnly ? 'desayunos' : 'productos'}.</p>
	{/if}
</div>
