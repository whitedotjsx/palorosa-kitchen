<script lang="ts">
	import { slugify, type ChoiceGroup } from '@palorosa-kitchen/core'
	import { recompute, editor } from '../../lib/catalog.svelte'
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

	function setAliases (value: string): string[] {
		return value
			.split(',')
			.map((alias) => alias.trim())
			.filter(Boolean)
	}
</script>

{#if !recipe}
	<p class="muted">Este producto no tiene receta.</p>
{:else}
	<p class="helper" style="margin:0 0 12px">
		Cada línea <b>suma</b> unidades a la lista de cocina. En una <b>elección</b>, cada opción es lo que
		puede decir el pedido; los alias son otras formas de escribirlo.
	</p>

	<div class="rx">
		{#each recipe.components as component, index (index)}
			{#if component.kind === 'fixed'}
				<div class="rx-line">
					<span class="stamp herb">Suma</span>
					<UnitSelect bind:value={component.unitId} onchange={recompute} />
					<input
						class="input qty"
						type="number"
						min="1"
						aria-label="Cantidad"
						bind:value={component.quantity}
						onchange={recompute}
					/>
					<button class="icon-x" title="Quitar" aria-label="Quitar" onclick={() => removeComponent(index)}>✕</button>
				</div>
			{:else}
				<div class="rx-group">
					<div class="rx-line">
						<span class="stamp honey">Elección</span>
						<input
							class="input"
							bind:value={component.label}
							onchange={recompute}
							placeholder="Nombre del grupo (p. ej. Bebida)"
						/>
						<label class="check nowrap">
							<input type="checkbox" bind:checked={component.required} onchange={recompute} />
							Obligatoria
						</label>
						<button class="icon-x" title="Quitar grupo" aria-label="Quitar grupo" onclick={() => removeComponent(index)}>✕</button>
					</div>

					{#each component.options as option, optionIndex (option.id)}
						<div class="rx-option">
							<div class="rx-line">
								<input
									class="input"
									bind:value={option.label}
									onchange={recompute}
									placeholder="Lo que dice el pedido"
								/>
								<button
									class="btn btn-ghost btn-sm"
									class:on={expandedAliases[option.id]}
									onclick={() => (expandedAliases[option.id] = !expandedAliases[option.id])}
								>
									Alias · {option.matchAliases.length}
								</button>
								<button class="icon-x" title="Quitar opción" aria-label="Quitar opción" onclick={() => removeOption(component, optionIndex)}>✕</button>
							</div>

							{#if expandedAliases[option.id]}
								<input
									class="input alias-input"
									value={option.matchAliases.join(', ')}
									onchange={(event) => {
										option.matchAliases = setAliases(event.currentTarget.value)
										recompute()
									}}
									placeholder="Variantes separadas por coma"
								/>
							{/if}

							{#each option.components as fixed, fixedIndex (fixedIndex)}
								<div class="rx-line nested">
									<span class="plus">+</span>
									<UnitSelect bind:value={fixed.unitId} onchange={recompute} />
									<input
										class="input qty"
										type="number"
										min="1"
										aria-label="Cantidad"
										bind:value={fixed.quantity}
										onchange={recompute}
									/>
									<button
										class="icon-x"
										title="Quitar"
										aria-label="Quitar"
										onclick={() => {
											option.components.splice(fixedIndex, 1)
											recompute()
										}}>✕</button
									>
								</div>
							{/each}
							<button class="link-add" onclick={() => addOptionUnit(component, optionIndex)}>+ unidad</button>
						</div>
					{/each}
					<button class="link-add" onclick={() => addOption(component)}>+ opción</button>
				</div>
			{/if}
		{/each}

		{#if recipe.components.length === 0}
			<p class="muted" style="margin:0">La receta está vacía.</p>
		{/if}
	</div>

	<div class="rx-actions">
		<button class="btn btn-kraft btn-sm" onclick={addFixed}>+ Unidad fija</button>
		<button class="btn btn-ghost btn-sm" onclick={addChoice}>+ Grupo de elección</button>
	</div>
{/if}

<style>
	.rx {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.rx-line {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.rx-line > .input:not(.qty) {
		flex: 1 1 10rem;
		min-width: 0;
	}
	.rx-line .stamp {
		flex: 0 0 76px;
		justify-content: center;
		text-align: center;
	}
	.qty {
		flex: 0 0 64px;
		width: 64px;
		text-align: center;
	}
	.nowrap {
		white-space: nowrap;
	}
	.rx-group {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px;
		border: 1px dashed var(--border-strong);
		border-radius: 10px;
		background: var(--surface-alt);
	}
	.rx-option {
		display: flex;
		flex-direction: column;
		gap: 7px;
		margin-left: 84px;
		padding: 9px 10px;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 9px;
	}
	.alias-input {
		font-size: 12.5px;
	}
	.nested .plus {
		flex: 0 0 18px;
		text-align: center;
		color: var(--text-muted);
		font-weight: 700;
	}
	.icon-x {
		flex: 0 0 30px;
		width: 30px;
		height: 30px;
		border: 1px solid var(--border);
		border-radius: 8px;
		background: transparent;
		color: var(--text-muted);
		cursor: pointer;
		font-size: 12px;
	}
	.icon-x:hover {
		border-color: var(--danger, #b4432f);
		color: var(--danger, #b4432f);
	}
	.link-add {
		align-self: flex-start;
		background: none;
		border: none;
		padding: 2px 0;
		font-family: var(--font-ui);
		font-size: 12.5px;
		font-weight: 600;
		color: var(--accent);
		cursor: pointer;
	}
	.link-add:hover {
		text-decoration: underline;
	}
	.btn.on {
		background: var(--accent-soft);
	}
	.rx-actions {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		margin-top: 12px;
	}
	@media (max-width: 699px) {
		.rx-option {
			margin-left: 0;
		}
		.rx-line {
			flex-wrap: wrap;
		}
		.rx-line .stamp {
			flex-basis: auto;
		}
	}
</style>
