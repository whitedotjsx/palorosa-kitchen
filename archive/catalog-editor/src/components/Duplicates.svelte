<script lang="ts">
	import { normalizeName, type KitchenUnit } from '@palorosa-kitchen/core'
	import { editor, mergeUnit } from '../lib/store.svelte'

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

<p class="muted">
	Pares de unidades activas con dos o más palabras significativas en común. "Misma receta" indica que
	aparecen juntas en un desayuno, la señal más fuerte de duplicado. Fusionar desactiva la origen, le
	agrega su nombre como alias a la destino y lo registra en overrides.
</p>

{#each pairs as pair (pair.a.id + pair.b.id)}
	<div class="pair" class:strong={pair.sameRecipe}>
		{#if pair.sameRecipe}<span class="badge">Misma receta</span>{/if}
		<div class="sides">
			<div class="side">
				<strong>{pair.a.name}</strong>
				<div class="muted small">{pair.a.id} · {usage.get(pair.a.id) ?? 0} usos</div>
				<div class="desc">{pair.a.description ?? ''}</div>
				<button onclick={() => mergeUnit(pair.a.id, pair.b.id)}>Fusionar A en B</button>
			</div>
			<div class="side">
				<strong>{pair.b.name}</strong>
				<div class="muted small">{pair.b.id} · {usage.get(pair.b.id) ?? 0} usos</div>
				<div class="desc">{pair.b.description ?? ''}</div>
				<button onclick={() => mergeUnit(pair.b.id, pair.a.id)}>Fusionar B en A</button>
			</div>
		</div>
		<div class="muted small">Común: {pair.common.join(', ')}</div>
	</div>
{/each}

{#if pairs.length === 0}
	<p>Sin pares sospechosos.</p>
{/if}

<style>
	.pair {
		border: 1px solid var(--line);
		border-radius: 6px;
		padding: 0.6rem;
		margin-bottom: 0.6rem;
	}
	.pair.strong {
		border-color: #b98a5e;
		background: #fdf6ec;
	}
	.sides {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.75rem;
		margin-bottom: 0.35rem;
	}
	.side {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.desc {
		font-size: 0.8rem;
		color: var(--muted);
		min-height: 1.2rem;
	}
	.badge {
		display: inline-block;
		font-size: 0.7rem;
		background: #b98a5e;
		color: #fff;
		border-radius: 4px;
		padding: 0.05rem 0.35rem;
		margin-bottom: 0.35rem;
	}
	.small {
		font-size: 0.7rem;
	}
</style>
