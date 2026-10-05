<script lang="ts">
	import { normalizeName, type KitchenUnit } from '@palorosa-kitchen/core'
	import { editor } from '../lib/store.svelte'

	let { value = $bindable('') }: { value?: string } = $props()

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

	function label (unit: KitchenUnit): string {
		return `${unit.name}${unit.active ? '' : ' (inactiva)'}${unit.isKitchen ? '' : ' (no cocina)'}`
	}

	function openList (): void {
		open = true
		query = ''
		highlight = Math.max(
			0,
			filtered.findIndex((unit) => unit.id === value)
		)
	}

	function close (): void {
		open = false
	}

	function select (unit: KitchenUnit): void {
		value = unit.id
		open = false
		query = ''
		input?.blur()
	}

	function clear (): void {
		value = ''
		open = false
		query = ''
		input?.blur()
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
			if (unit) select(unit)
		} else if (event.key === 'Escape') {
			close()
			input?.blur()
		}
	}
</script>

<div class="combo">
	<input
		bind:this={input}
		value={open ? query : selected ? label(selected) : ''}
		placeholder="Buscar unidad..."
		onfocus={openList}
		oninput={(event) => {
			query = event.currentTarget.value
			open = true
			highlight = 0
		}}
		onkeydown={onKeydown}
		onblur={() => setTimeout(close, 120)}
	/>

	{#if open}
		<ul class="list">
			<li>
				<button type="button" class="option clear" onmousedown={(event) => event.preventDefault()} onclick={clear}>
					(sin unidad)
				</button>
			</li>
			{#each filtered as unit, index (unit.id)}
				<li>
					<button
						type="button"
						class="option"
						class:highlight={index === highlight}
						onmousedown={(event) => event.preventDefault()}
						onclick={() => select(unit)}
					>
						{label(unit)}
					</button>
				</li>
			{/each}
			{#if filtered.length === 0}
				<li class="empty">Sin coincidencias</li>
			{/if}
		</ul>
	{/if}
</div>

<style>
	.combo {
		position: relative;
		min-width: 13rem;
		flex: 1 1 13rem;
	}
	.combo input {
		width: 100%;
	}
	.list {
		position: absolute;
		z-index: 30;
		top: calc(100% + 2px);
		left: 0;
		right: 0;
		margin: 0;
		padding: 0.2rem;
		list-style: none;
		background: #fff;
		border: 1px solid var(--line);
		border-radius: 6px;
		box-shadow: 0 6px 18px rgba(0, 0, 0, 0.12);
		max-height: 17rem;
		overflow: auto;
	}
	.option {
		display: block;
		width: 100%;
		text-align: left;
		background: transparent;
		border: none;
		padding: 0.25rem 0.4rem;
		border-radius: 4px;
	}
	.option:hover,
	.option.highlight {
		background: var(--accent-soft);
	}
	.clear {
		color: var(--muted);
	}
	.empty {
		padding: 0.3rem 0.4rem;
		color: var(--muted);
		font-size: 0.8rem;
	}
</style>
