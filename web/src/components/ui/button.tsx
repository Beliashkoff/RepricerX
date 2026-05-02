import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-xl text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#ffcc00] disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        default: 'bg-[#ffcc00] text-[#111111] hover:bg-[#f2c200]',
        secondary: 'bg-[#f2f3f5] text-[#111111] hover:bg-[#e8e9eb]',
        black: 'bg-[#111111] text-white hover:bg-[#333333]',
        ghost: 'text-[#111111] hover:bg-[#f5f5f5]',
        destructive: 'bg-red-100 text-red-700 hover:bg-red-200',
        outline: 'border border-[#e6e6e6] bg-white text-[#111111] hover:bg-[#f5f5f5]',
      },
      size: {
        default: 'h-11 px-6 py-2.5',
        sm: 'h-9 px-4 py-2 text-xs',
        lg: 'h-13 px-8 py-3 text-base',
        icon: 'h-10 w-10',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  }
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button'
    return (
      <Comp className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />
    )
  }
)
Button.displayName = 'Button'

export { Button, buttonVariants }
