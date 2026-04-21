import { cn } from '@/lib/utils'

interface BadgeProps {
  variant?: 'default' | 'secondary' | 'destructive' | 'outline' | 'success' | 'warning'
  className?: string
  children: React.ReactNode
}

const variantClasses: Record<NonNullable<BadgeProps['variant']>, string> = {
  default: 'border-transparent bg-primary text-primary-foreground',
  secondary: 'border-transparent bg-secondary text-secondary-foreground',
  destructive: 'border-transparent bg-destructive text-destructive-foreground',
  outline: 'text-foreground',
  success: 'border-transparent bg-success text-primary-foreground',
  warning: 'border-transparent bg-warning text-primary-foreground',
}

export function Badge({ variant = 'default', className, children }: BadgeProps) {
  return (
    <div
      className={cn(
        'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors',
        variantClasses[variant],
        className,
      )}
    >
      {children}
    </div>
  )
}
