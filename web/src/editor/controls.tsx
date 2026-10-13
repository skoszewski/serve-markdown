import {
  Button as HeadlessButton,
  Input,
  Listbox,
  ListboxButton,
  ListboxOption,
  ListboxOptions,
  type ButtonProps,
  type InputProps,
} from '@headlessui/react';

/** Light controls are used on white and light backgrounds, bar controls on the top bar. */
type Surface = 'light' | 'bar';

/**
 * Joins CSS classes, skipping the empty ones.
 *
 * @param classes CSS classes.
 * @returns The class attribute value.
 */
function classNames(...classes: (string | undefined | false)[]): string {
  return classes.filter(Boolean).join(' ');
}

/**
 * Headless UI button of the editor: primary and secondary on light backgrounds, bar-primary and bar on the top bar.
 *
 * @param props.variant Visual variant, secondary by default.
 * @param props.className Additional CSS class.
 * @returns The button.
 */
export function Button({
  variant = 'secondary',
  className,
  ...props
}: ButtonProps & {variant?: 'primary' | 'secondary' | 'bar' | 'bar-primary'; className?: string}) {
  return <HeadlessButton className={classNames('button', `button-${variant}`, className)} {...props} />;
}

/**
 * Headless UI text input of the editor.
 *
 * @param props.surface Background the input is placed on, light by default.
 * @param props.className Additional CSS class.
 * @returns The input.
 */
export function TextInput({surface = 'light', className, ...props}: InputProps & {surface?: Surface; className?: string}) {
  return <Input className={classNames('text-input', `text-input-${surface}`, className)} {...props} />;
}

/**
 * Headless UI dropdown list selecting one of the options.
 *
 * @param props.value Selected option value.
 * @param props.options Option values mapped to their labels.
 * @param props.onChange Receives the selected option value.
 * @param props.label Accessible name of the dropdown.
 * @param props.disabled Disables the dropdown.
 * @param props.surface Background the dropdown is placed on, light by default.
 * @returns The dropdown.
 */
export function Dropdown({
  value,
  options,
  onChange,
  label,
  disabled,
  surface = 'light',
}: {
  value: string;
  options: Record<string, string>;
  onChange: (value: string) => void;
  label: string;
  disabled?: boolean;
  surface?: Surface;
}) {
  return (
    <Listbox value={value} onChange={onChange} disabled={disabled}>
      <ListboxButton className={classNames('dropdown', `dropdown-${surface}`)} aria-label={label}>
        <span>{options[value] ?? value}</span>
        <svg viewBox="0 0 12 12" fill="none" aria-hidden="true">
          <path d="M3 4.5L6 7.5L9 4.5" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </ListboxButton>
      <ListboxOptions anchor="bottom start" className="dropdown-options">
        {Object.entries(options).map(([optionValue, optionLabel]) => (
          <ListboxOption key={optionValue} value={optionValue} className="dropdown-option">
            {optionLabel}
          </ListboxOption>
        ))}
      </ListboxOptions>
    </Listbox>
  );
}
