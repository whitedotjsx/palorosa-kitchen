<script lang="ts">
  interface Props {
    title: string
    text: string
  }
  let { title, text }: Props = $props()
</script>

<div class="card">
  <h2>{title}</h2>
  <p class="muted">{text}</p>
</div>
