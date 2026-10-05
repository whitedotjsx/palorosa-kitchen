<script lang="ts">
	import { normalizeName, type KitchenUnit } from '@palorosa-kitchen/core'
	import { editor, mergeUnit } from '../../lib/catalog.svelte'

	const STOP = new Set([
		'de',
		'con',
		'y',
		'el',
		'la',
		'los',
		'las',
		'un',
		'una',
		'para',
		'en',
		'al',
		'del',
		'o',
		'su',
		'mini',
		'grande',
		'pequeno',
		'pequena',
		'pequenos',
		'pequenas',
		'porcion',
		'porcione',
	])

	function tokens (name: string): Set<string> {
		return new Set(
			normalizeName(name)
				.split(' ')
				.filter((word) => word && !STOP.has(word))
		)
	}

	const usage = $derived.by(() => {
		const counts = new Map<string, number>()
		for (const recipe of editor.catalog?.recipes ?? []) {
			for (const component of recipe.components) {
				if (component.kind === 'fixed') {
					counts.set(component.unitId, (counts.get(component.unitId) ?? 0) + 1)
					continue
				}
				for (const option of component.options) {
					for (const fixed of option.components) {
						counts.set(fixed.unitId, (counts.get(fixed.unitId) ?? 0) + 1)
					}
				}
			}
		}
		return counts
	})

	interface Pair {
		a: KitchenUnit
		b: KitchenUnit
		common: string[]
		sameRecipe: boolean
	}

	const pairs = $derived.by(() => {
		const units = (editor.catalog?.units ?? []).filter(
			(unit) => unit.active && !editor.merges[unit.id]
		)
		const recipesByUnit = new Map<string, Set<string>>()
		for (const recipe of editor.catalog?.recipes ?? []) {
			for (const component of recipe.components) {
				const ids =
					component.kind === 'fixed'
						? [component.unitId]
						: component.options.flatMap((option) => option.components.map((fixed) => fixed.unitId))
				for (const id of ids) {
					const set = recipesByUnit.get(id) ?? new Set<string>()
					set.add(recipe.productId)
					recipesByUnit.set(id, set)
				}
			}
		}
		const result: Pair[] = []
		for (let i = 0; i < units.length; i += 1) {
			for (let j = i + 1; j < units.length; j += 1) {
				const a = units[i]
				const b = units[j]
				if (!a || !b) continue
				const common = [...tokens(a.name)].filter((token) => tokens(b.name).has(token))
				if (common.length < 2) continue
				const recipesA = recipesByUnit.get(a.id) ?? new Set<string>()
				const sameRecipe = [...(recipesByUnit.get(b.id) ?? [])].some((id) => recipesA.has(id))
				result.push({ a, b, common, sameRecipe })
			}
		}
		return result.sort(
			(first, second) =>
				Number(second.sameRecipe) - Number(first.sameRecipe) ||
				second.common.length - first.common.length
		)
	})
</script>

<div class="note" style="margin-bottom:18px">
	<svg viewBox="0 0 24 24"><path d="M12 8v5M12 16h.01" /><circle cx="12" cy="12" r="9" /></svg>
	<span>
		Unidades activas con dos o más palabras en común. <b>Misma receta</b> significa que salen juntas en un
		desayuno: la señal más fuerte de duplicado. Al fusionar, la unidad que eliges desaparece y su nombre
		queda como alias de la otra.
	</span>
</div>

{#if pairs.length === 0}
	<div class="recipe"><p class="muted" style="margin:0">No hay unidades sospechosas de estar duplicadas.</p></div>
{:else}
	<p class="helper" style="margin:0 0 12px">{pairs.length} pares para revisar</p>
	<div class="dup-list">
		{#each pairs as pair (pair.a.id + pair.b.id)}
			<article class="recipe dup">
				<div class="dup-head">
					{#if pair.sameRecipe}<span class="stamp honey">Misma receta</span>{/if}
					<div class="chips">
						{#each pair.common as word (word)}<span class="chip mono">{word}</span>{/each}
					</div>
				</div>
				<div class="dup-sides">
					{#each [{ unit: pair.a, other: pair.b }, { unit: pair.b, other: pair.a }] as { unit, other } (unit.id)}
						<div class="dup-side">
							<b>{unit.name}</b>
							<span class="helper" style="margin:0">{usage.get(unit.id) ?? 0} recetas · {unit.id}</span>
							{#if unit.description}<p>{unit.description}</p>{/if}
							<button class="btn btn-kraft btn-sm" onclick={() => mergeUnit(other.id, unit.id)}>Quedarme con esta</button>
						</div>
					{/each}
				</div>
			</article>
		{/each}
	</div>
{/if}

<style>
	.dup-list {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	.dup {
		margin: 0;
	}
	.dup-head {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
		margin-bottom: 12px;
	}
	.dup-sides {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 12px;
	}
	.dup-side {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 12px;
		border: 1px dashed var(--border-strong);
		border-radius: 10px;
		background: var(--surface-alt);
		min-width: 0;
	}
	.dup-side b {
		font-family: var(--font-display);
		font-weight: 600;
		font-size: 14.5px;
	}
	.dup-side p {
		margin: 4px 0 6px;
		font-size: 12.5px;
		color: var(--text-muted);
		line-height: 1.45;
	}
	.dup-side .btn {
		margin-top: auto;
		align-self: flex-start;
	}
	@media (max-width: 699px) {
		.dup-sides {
			grid-template-columns: 1fr;
		}
	}
</style>
