<script lang="ts">
	import { normalizeName, type KitchenUnit } from '@palorosa-kitchen/core'
	import { editor } from '../../lib/catalog.svelte'

	let {
		value = $bindable(''),
		onchange,
		placeholder = 'Buscar unidad…',
	}: { value?: string; onchange?: (id: string) => void; placeholder?: string } = $props()

	let query = $state('')
	let open = $state(false)
	let highlight = $state(0)
	let input = $state<HTMLInputElement | null>(null)

	const units = $derived(
		[...(editor.catalog?.units ?? [])].sort((a, b) => a.name.localeCompare(b.name))
	)

	const selected = $derived(units.find((unit) => unit.id === value))

	const filtered = $derived.by(() => {
		const normalized = normalizeName(query)
		if (!normalized) return units.slice(0, 60)
		return units
			.filter(
				(unit) => normalizeName(unit.name).includes(normalized) || unit.id.includes(normalized)
			)
			.slice(0, 60)
	})

	function openList (): void {
		open = true
		query = ''
		highlight = Math.max(
			0,
			filtered.findIndex((unit) => unit.id === value)
		)
	}

	function pick (id: string): void {
		value = id
		open = false
		query = ''
		input?.blur()
		onchange?.(id)
	}

	function onKeydown (event: KeyboardEvent): void {
		if (event.key === 'ArrowDown') {
			event.preventDefault()
			open = true
			highlight = Math.min(highlight + 1, filtered.length - 1)
		} else if (event.key === 'ArrowUp') {
			event.preventDefault()
			highlight = Math.max(highlight - 1, 0)
		} else if (event.key === 'Enter') {
			event.preventDefault()
			const unit = filtered[highlight]
			if (unit) pick(unit.id)
		} else if (event.key === 'Escape') {
			open = false
			input?.blur()
		}
	}

	function flags (unit: KitchenUnit): string {
		return [unit.active ? '' : 'inactiva', unit.isKitchen ? '' : 'no cocina'].filter(Boolean).join(' · ')
	}
</script>

<div class="combo" class:empty={!selected}>
	<input
		class="input"
		bind:this={input}
		value={open ? query : (selected?.name ?? '')}
		{placeholder}
		onfocus={openList}
		oninput={(event) => {
			query = event.currentTarget.value
			open = true
			highlight = 0
		}}
		onkeydown={onKeydown}
		onblur={() => setTimeout(() => (open = false), 120)}
	/>

	{#if open}
		<ul class="combo-list" role="listbox">
			{#if value}
				<li>
					<button type="button" class="opt clear" onmousedown={(event) => event.preventDefault()} onclick={() => pick('')}>
						Quitar unidad
					</button>
				</li>
			{/if}
			{#each filtered as unit, index (unit.id)}
				<li>
					<button
						type="button"
						class="opt"
						class:hl={index === highlight}
						class:sel={unit.id === value}
						onmousedown={(event) => event.preventDefault()}
						onclick={() => pick(unit.id)}
					>
						<span>{unit.name}</span>
						{#if flags(unit)}<small>{flags(unit)}</small>{/if}
					</button>
				</li>
			{/each}
			{#if filtered.length === 0}
				<li class="none">Sin coincidencias</li>
			{/if}
		</ul>
	{/if}
</div>

<style>
	.combo {
		position: relative;
		flex: 1 1 12rem;
		min-width: 0;
	}
	.combo-list {
		position: absolute;
		z-index: 40;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		margin: 0;
		padding: 4px;
		list-style: none;
		background: var(--surface);
		border: 1px solid var(--border-strong);
		border-radius: 10px;
		box-shadow: 3px 4px 0 rgba(138, 90, 43, 0.12), 0 8px 24px rgba(106, 75, 32, 0.12);
		max-height: 18rem;
		overflow: auto;
	}
	.opt {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 8px;
		width: 100%;
		text-align: left;
		background: transparent;
		border: none;
		padding: 7px 9px;
		border-radius: 7px;
		font-family: var(--font-ui);
		font-size: 13px;
		color: var(--text);
		cursor: pointer;
	}
	.opt small {
		font-size: 11px;
		color: var(--text-muted);
		white-space: nowrap;
	}
	.opt:hover,
	.opt.hl {
		background: var(--surface-alt);
	}
	.opt.sel {
		font-weight: 600;
		color: var(--accent);
	}
	.clear {
		color: var(--text-muted);
		border-bottom: 1px dashed var(--border);
		border-radius: 7px 7px 0 0;
	}
	.none {
		padding: 8px 9px;
		color: var(--text-muted);
		font-size: 12.5px;
	}
</style>
