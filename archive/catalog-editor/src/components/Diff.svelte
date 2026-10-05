<script lang="ts">
	import { download, recompute, editor } from '../lib/store.svelte'

	const patch = $derived(editor.patch)

	const summary = $derived({
		units: Object.keys(patch.units ?? {}).length,
		products: Object.keys(patch.products ?? {}).length,
		containers: Object.keys(patch.containers ?? {}).length,
		recipes: Object.keys(patch.recipes ?? {}).length,
		merges: Object.keys(patch.unitMerges ?? {}).length,
	})
</script>

<div class="row">
	<button onclick={recompute}>Recalcular</button>
	<button onclick={() => download('overrides-patch.json', editor.patch)}>
		Descargar patch de overrides
	</button>
	<button onclick={() => download('seed.json', editor.catalog)}>Descargar catálogo completo</button>
</div>

<p>
	{summary.units} unidades · {summary.products} productos · {summary.recipes} recetas · {summary.containers}
	contenedores · {summary.merges} fusiones
</p>

<pre>{JSON.stringify(editor.patch, null, 2)}</pre>

<style>
	.row {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 0.5rem;
	}
	pre {
		background: var(--panel);
		padding: 0.75rem;
		border-radius: 6px;
		max-height: 65vh;
		overflow: auto;
		font-size: 0.8rem;
	}
</style>
