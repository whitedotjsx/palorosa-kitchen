<script lang="ts">
  import { labels } from '../lib/labels'
  import { bogotaDate, day } from '../lib/day.svelte'

  const today = $derived(bogotaDate(0))
  const tomorrow = $derived(bogotaDate(1))
  const other = $derived(day.date !== today && day.date !== tomorrow)
</script>

<div class="daypicker">
  <div class="datechips" role="group" aria-label={labels.day.label}>
    <button type="button" class:active={day.date === today} onclick={() => (day.date = today)}>{labels.day.today}</button>
    <button type="button" class:active={day.date === tomorrow} onclick={() => (day.date = tomorrow)}>{labels.day.tomorrow}</button>
  </div>
  <input
    class="input date-input"
    class:active={other}
    type="date"
    aria-label={labels.day.pick}
    value={day.date}
    onchange={(event) => { const value = event.currentTarget.value; if (value) day.date = value }}
  />
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
</style>
