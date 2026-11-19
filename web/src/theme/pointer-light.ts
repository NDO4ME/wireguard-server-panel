// A quiet light follows the pointer on cards: one delegated listener, one
// style write per frame, hover devices only, never with reduced motion.
export function initPointerLight(): void {
  if (!matchMedia('(hover: hover)').matches || matchMedia('(prefers-reduced-motion: reduce)').matches) return
  let raf = 0
  let el: HTMLElement | null = null
  let px = 0
  let py = 0
  document.addEventListener(
    'pointermove',
    (e) => {
      const card = (e.target as HTMLElement | null)?.closest?.('.panel, .vital') as HTMLElement | null
      if (!card) return
      el = card
      px = e.clientX
      py = e.clientY
      if (raf) return
      raf = requestAnimationFrame(() => {
        raf = 0
        if (!el) return
        const r = el.getBoundingClientRect()
        el.setAttribute('data-lit', '')
        el.style.setProperty('--mx', `${px - r.left}px`)
        el.style.setProperty('--my', `${py - r.top}px`)
      })
    },
    { passive: true },
  )
}
