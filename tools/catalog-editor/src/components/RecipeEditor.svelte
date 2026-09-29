<script lang="ts">
	import { slugify, type ChoiceGroup } from '@palorosa-kitchen/core'
	import { recompute, editor } from '../lib/store.svelte'
	import UnitSelect from './UnitSelect.svelte'

	let { productId }: { productId: string } = $props()

	let expandedAliases = $state<Record<string, boolean>>({})

	const recipe = $derived(editor.catalog?.recipes.find((item) => item.productId === productId))

	function addFixed (): void {
		recipe?.components.push({ kind: 'fixed', unitId: '', quantity: 1 })
		recompute()
	}

	function addChoice (): void {
		recipe?.components.push({
			kind: 'choice',
			label: 'Nueva elección',
			required: true,
			options: [],
		})
		recompute()
	}

	function removeComponent (index: number): void {
		recipe?.components.splice(index, 1)
		recompute()
	}

	function addOption (group: ChoiceGroup): void {
		group.options.push({
			id: `${slugify(group.label)}-${group.options.length + 1}`,
			label: 'Nueva opción',
			matchAliases: [],
			components: [],
		})
		recompute()
	}

	function addOptionUnit (group: ChoiceGroup, optionIndex: number): void {
		group.options[optionIndex]?.components.push({ kind: 'fixed', unitId: '', quantity: 1 })
		recompute()
	}

	function removeOption (group: ChoiceGroup, optionIndex: number): void {
		group.options.splice(optionIndex, 1)
		recompute()
	}
</script>

{#if !recipe}
	<p class="muted">Sin receta.</p>
{:else}
	<p class="hint">
		"Suma" son las unidades que este componente agrega a la lista de cocina, con su cantidad. En una
		elección, "Texto" es lo que dice el pedido y "Alias" las variantes con las que puede venir.
	</p>

	<div class="row">
		<button onclick={addFixed}>+ Unidad fija</button>
		<button onclick={addChoice}>+ Grupo de elección</button>
	</div>

	{#each recipe.components as component, index (index)}
		<div class="component">
			{#if component.kind === 'fixed'}
				<div class="row">
					<span class="tag">Suma</span>
					<UnitSelect bind:value={component.unitId} />
					<span class="caption">x</span>
					<input
						type="number"
						min="1"
						class="qty"
						bind:value={component.quantity}
						onchange={recompute}
					/>
					<button class="danger" onclick={() => removeComponent(index)}>Quitar</button>
				</div>
			{:else}
				<div class="group">
					<div class="row">
						<span class="tag">Elección</span>
						<input
							class="wide"
							bind:value={component.label}
							onchange={recompute}
							placeholder="Etiqueta del grupo"
						/>
						<label class="check">
							<input type="checkbox" bind:checked={component.required} onchange={recompute} />
							Obligatoria
						</label>
						<button onclick={() => addOption(component)}>+ Opción</button>
						<button class="danger" onclick={() => removeComponent(index)}>Quitar grupo</button>
					</div>

					{#each component.options as option, optionIndex (option.id)}
						<div class="option">
							<div class="row">
								<span class="tag">Texto</span>
								<input
									class="wide"
									bind:value={option.label}
									onchange={recompute}
									placeholder="Lo que dice el pedido"
								/>
								<button
									class="ghost"
									onclick={() =>
										(expandedAliases[option.id] = !expandedAliases[option.id])}
								>
									Alias ({option.matchAliases.length})
								</button>
								<button onclick={() => addOptionUnit(component, optionIndex)}>+ Unidad</button>
								<button class="danger" onclick={() => removeOption(component, optionIndex)}>
									Quitar opción
								</button>
							</div>

							{#if expandedAliases[option.id]}
								<div class="row">
									<span class="tag">Alias</span>
									<input
										class="wide"
										value={option.matchAliases.join(', ')}
										onchange={(event) => {
											option.matchAliases = event.currentTarget.value
												.split(',')
												.map((alias) => alias.trim())
												.filter(Boolean)
											recompute()
										}}
										placeholder="Variantes separadas por coma"
									/>
								</div>
							{/if}

							{#each option.components as fixed, fixedIndex (fixedIndex)}
								<div class="row nested">
									<span class="tag">Suma</span>
									<UnitSelect bind:value={fixed.unitId} />
									<span class="caption">x</span>
									<input
										type="number"
										min="1"
										class="qty"
										bind:value={fixed.quantity}
										onchange={recompute}
									/>
									<button
										class="danger"
										onclick={() => {
											option.components.splice(fixedIndex, 1)
											recompute()
										}}
									>
										Quitar
									</button>
								</div>
							{/each}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/each}
{/if}

<style>
	.hint {
		font-size: 0.8rem;
		color: var(--muted);
		margin: 0 0 0.5rem;
	}
	.row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		flex-wrap: wrap;
	}
	.component {
		border-bottom: 1px solid var(--line);
		padding: 0.5rem 0;
	}
	.group {
		background: var(--panel);
		padding: 0.5rem;
		border-radius: 6px;
	}
	.option {
		margin: 0.5rem 0 0.5rem 1rem;
		padding-left: 0.75rem;
		border-left: 2px solid var(--line);
	}
	.nested {
		margin-top: 0.35rem;
	}
	.tag {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--muted);
		min-width: 4rem;
	}
	.caption {
		color: var(--muted);
	}
	.qty {
		width: 4rem;
	}
	.wide {
		flex: 1;
		min-width: 12rem;
	}
	.check {
		display: flex;
		gap: 0.25rem;
		align-items: center;
		font-size: 0.85rem;
	}
	.ghost {
		background: transparent;
		border-style: dashed;
		color: var(--muted);
	}
	.danger {
		color: #a33;
	}
</style>
