import { useId } from 'react'

export function Switch({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label?: string
  disabled?: boolean
}) {
  const id = useId()
  return (
    <label htmlFor={id} className={`relative inline-flex items-center ${disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'}`}>
      <input
        id={id}
        type="checkbox"
        className="peer sr-only"
        checked={checked}
        disabled={disabled}
        onChange={(e) => !disabled && onChange(e.target.checked)}
      />
      <div className="h-6 w-11 rounded-full bg-slate-300 transition-colors peer-checked:bg-blue-600 dark:bg-slate-700" />
      <span className="absolute left-1 top-1 h-4 w-4 rounded-full bg-white transition-transform peer-checked:translate-x-5" />
      {label && <span className="ml-3 text-sm font-medium text-slate-700 dark:text-slate-300">{label}</span>}
    </label>
  )
}
