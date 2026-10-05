import { mount } from 'svelte'
import App from './App.svelte'
import './app.css'

// The desktop shell injects this flag before the document loads. Set the class
// here so the fixed title bar offset applies before the first paint.
if ((window as { __palorosaDesktop?: boolean }).__palorosaDesktop) {
  document.documentElement.classList.add('is-desktop')
}

const target = document.getElementById('app')
if (!target) throw new Error('Missing #app element')

mount(App, { target })

// Register the service worker so the panel is installable (Phase 4). It only
// runs over http(s); the file build stays offline and skips it.
if ('serviceWorker' in navigator && location.protocol.startsWith('http')) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {
      // Registration is best effort; the panel still works without it.
    })
  })
}
