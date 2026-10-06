<script lang="ts">
  import { labels } from '../lib/labels'
  import { bogotaDate, day, maxRangeDays, shiftDate } from '../lib/day.svelte'

  interface Props {
    /** Offers the several-days mode (Lista only). */
    range?: boolean
  }
  let { range = false }: Props = $props()

  const today = $derived(bogotaDate(0))
  const tomorrow = $derived(bogotaDate(1))
  const rangeMode = $derived(range && day.range)
  const other = $derived(!rangeMode && day.date !== today && day.date !== tomorrow)
  const maxTo = $derived(shiftDate(day.from, maxRangeDays - 1))

  function pickDay (date: string) {
    day.range = false
    day.date = date
  }

  function toggleRange () {
    if (!day.range) {
      // The range starts at the day on screen, so switching back and forth
      // does not jump to an unrelated week.
      day.from = day.date
      day.to = clampTo(day.to, day.from)
    }
    day.range = !day.range
  }

  function clampTo (value: string, from: string): string {
    const max = shiftDate(from, maxRangeDays - 1)
    return value < from ? from : value > max ? max : value
  }

  function setFrom (value: string) {
    if (!value) return
    day.from = value
    day.to = clampTo(day.to, value)
  }

  function setTo (value: string) {
    if (!value) return
    day.to = clampTo(value, day.from)
  }
</script>

<div class="daypicker">
  <div class="datechips" role="group" aria-label={labels.day.label}>
    <button type="button" class:active={!rangeMode && day.date === today} onclick={() => pickDay(today)}>{labels.day.today}</button>
    <button type="button" class:active={!rangeMode && day.date === tomorrow} onclick={() => pickDay(tomorrow)}>{labels.day.tomorrow}</button>
    {#if range}
      <button type="button" class:active={rangeMode} title={labels.day.rangeHint} onclick={toggleRange}>{labels.day.range}</button>
    {/if}
  </div>
  {#if rangeMode}
    <label class="range-field">
      <span>{labels.day.from}</span>
      <input class="input date-input" type="date" value={day.from} max={maxTo} onchange={(event) => setFrom(event.currentTarget.value)} />
    </label>
    <label class="range-field">
      <span>{labels.day.to}</span>
      <input class="input date-input" type="date" value={day.to} min={day.from} max={maxTo} onchange={(event) => setTo(event.currentTarget.value)} />
    </label>
  {:else}
    <input
      class="input date-input"
      class:active={other}
      type="date"
      aria-label={labels.day.pick}
      value={day.date}
      onchange={(event) => { const value = event.currentTarget.value; if (value) day.date = value }}
    />
  {/if}
</div>

<style>
  .daypicker {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .date-input {
    width: auto;
    min-width: 0;
    padding: 6px 10px;
    font-size: 13px;
  }
  .date-input.active {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }
  .range-field {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12.5px;
    color: var(--label);
  }
</style>
