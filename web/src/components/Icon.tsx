import sprite from '../theme/icons.svg?raw'

/** Renders the shared SVG sprite once (symbols are referenced by <Icon />). */
export function IconSprite() {
  return <div hidden aria-hidden="true" dangerouslySetInnerHTML={{ __html: sprite }} />
}

interface IconProps {
  name: string
  className?: string
}

export function Icon({ name, className }: IconProps) {
  return (
    <svg className={className ? `ic ${className}` : 'ic'} aria-hidden="true">
      <use href={`#${name}`} />
    </svg>
  )
}
