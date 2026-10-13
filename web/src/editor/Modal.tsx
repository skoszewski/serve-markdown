import {
  Checkbox,
  Description,
  Dialog as HeadlessDialog,
  DialogBackdrop,
  DialogPanel,
  DialogTitle,
  Field,
  Label,
  Radio,
  RadioGroup,
} from '@headlessui/react';
import {useState, type ReactNode} from 'react';
import {Button, Dropdown, TextInput} from './controls';
import {appearances, palettes, type Appearance, type Palette} from './theme';

/** Document properties kept in the front matter. */
export interface Properties {
  title?: string;
  author?: string;
  created?: string;
  updated?: string;
  tags?: string[];
}

/** Template description from the front matter of a template. */
export interface Template {
  name: string;
  title: string;
  author: string;
  description: string;
}

/** Formats the documents are read in: plain Markdown with its HTML kept as written, or MDX with HTML read as JSX. */
export const formats = {
  markdown: 'Markdown',
  mdx: 'MDX (Markdown with JSX)',
};

export type Format = keyof typeof formats;

/** Editor settings kept in the browser. */
export interface Settings {
  buttonTextLabels: boolean;
  saveButtons: boolean;
  gitControls: boolean;
  palette: Palette;
  appearance: Appearance;
  format: Format;
}

/** Content of a modal dialog, with a text field when value is set. */
export interface ModalRequest {
  title: string;
  message: string;
  value?: string;
  confirmLabel: string;
}

/**
 * Headless UI modal dialog, open while mounted.
 *
 * @param props.className Additional CSS class of the panel.
 * @param props.onCancel Called on Escape and on a click outside the panel, the dialog stays open without it.
 * @param props.children Panel content.
 * @returns The dialog.
 */
function Dialog({className, onCancel, children}: {className?: string; onCancel?: () => void; children: ReactNode}) {
  return (
    <HeadlessDialog open onClose={() => onCancel?.()} className="modal-root">
      <DialogBackdrop className="modal-backdrop" />
      <div className="modal-container">
        <DialogPanel className={className ? `modal ${className}` : 'modal'}>{children}</DialogPanel>
      </div>
    </HeadlessDialog>
  );
}

/**
 * Confirm and Cancel buttons of a dialog form.
 *
 * @param props.confirmLabel Label of the submit button.
 * @param props.onCancel Called by the Cancel button.
 * @param props.focusConfirm Focuses the submit button.
 * @returns The buttons.
 */
function Buttons({confirmLabel, onCancel, focusConfirm}: {confirmLabel: string; onCancel: () => void; focusConfirm?: boolean}) {
  return (
    <div className="modal-buttons">
      <Button type="submit" variant="primary" autoFocus={focusConfirm}>
        {confirmLabel}
      </Button>
      <Button onClick={onCancel}>Cancel</Button>
    </div>
  );
}

/**
 * Text field of a dialog form.
 *
 * @param props.label Field label.
 * @param props.value Field value.
 * @param props.onChange Receives the new value, the field is read-only without it.
 * @param props.autoFocus Focuses the field.
 * @returns The field.
 */
function TextField({label, value, onChange, autoFocus}: {label: string; value: string; onChange?: (value: string) => void; autoFocus?: boolean}) {
  return (
    <Field className="field">
      <Label>{label}</Label>
      <TextInput
        value={value}
        readOnly={!onChange}
        autoFocus={autoFocus}
        onChange={(event) => onChange?.(event.target.value)}
      />
    </Field>
  );
}

/**
 * Modal dialog shown while a file loads, closed only by unmounting it.
 *
 * @param props.message Loading message.
 * @returns The dialog.
 */
export function Loading({message}: {message: string}) {
  return (
    <Dialog className="loading">
      <div className="spinner" />
      <p>{message}</p>
    </Dialog>
  );
}

/**
 * Modal dialog asking for a confirmation or a text value.
 *
 * @param props.request Dialog content.
 * @param props.onClose Receives the text value, an empty string without a text field, or null when cancelled.
 * @returns The dialog.
 */
export function Modal({request, onClose}: {request: ModalRequest; onClose: (result: string | null) => void}) {
  const [value, setValue] = useState(request.value ?? '');

  return (
    <Dialog onCancel={() => onClose(null)}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onClose(request.value === undefined ? '' : value.trim() || null);
        }}
      >
        <DialogTitle>{request.title}</DialogTitle>
        {request.value === undefined ? (
          <Description>{request.message}</Description>
        ) : (
          <TextField label={request.message} value={value} onChange={setValue} autoFocus />
        )}
        <Buttons confirmLabel={request.confirmLabel} onCancel={() => onClose(null)} focusConfirm={request.value === undefined} />
      </form>
    </Dialog>
  );
}

/**
 * Modal dialog choosing one of several actions, or cancelling.
 *
 * @param props.title Dialog title.
 * @param props.message Dialog message.
 * @param props.choices Action values mapped to their button labels, the first one is the primary action.
 * @param props.onClose Receives the chosen action value, or null when cancelled.
 * @returns The dialog.
 */
export function Choice({
  title,
  message,
  choices,
  onClose,
}: {
  title: string;
  message: string;
  choices: Record<string, string>;
  onClose: (result: string | null) => void;
}) {
  return (
    <Dialog onCancel={() => onClose(null)}>
      <DialogTitle>{title}</DialogTitle>
      <Description>{message}</Description>
      <div className="modal-buttons">
        {Object.entries(choices).map(([value, label], index) => (
          <Button key={value} variant={index === 0 ? 'primary' : 'secondary'} autoFocus={index === 0} onClick={() => onClose(value)}>
            {label}
          </Button>
        ))}
        <Button onClick={() => onClose(null)}>Cancel</Button>
      </div>
    </Dialog>
  );
}

/**
 * Modal dialog selecting a template and the path of the new file.
 *
 * @param props.templates Available templates.
 * @param props.path Initial path of the new file.
 * @param props.onClose Receives the template file name and the path, or null when cancelled.
 * @returns The dialog.
 */
export function TemplateModal({
  templates,
  path,
  onClose,
}: {
  templates: Template[];
  path: string;
  onClose: (result: {template: string; path: string} | null) => void;
}) {
  const [template, setTemplate] = useState(templates[0]?.name);
  const [value, setValue] = useState(path);

  return (
    <Dialog className="templates" onCancel={() => onClose(null)}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (template && value.trim()) {
            onClose({template, path: value.trim()});
          }
        }}
      >
        <DialogTitle>New file from Template</DialogTitle>
        {templates.length === 0 && <Description>The templates directory has no templates.</Description>}
        <RadioGroup value={template} onChange={setTemplate} aria-label="Template" className="template-list">
          {templates.map((item) => (
            <Radio key={item.name} value={item.name} className="template">
              <strong>{item.title}</strong>
              {item.author && <small>{item.author}</small>}
              <span>{item.description}</span>
            </Radio>
          ))}
        </RadioGroup>
        <TextField label="Path of the new file, ending with .md or .markdown" value={value} onChange={setValue} autoFocus />
        <Buttons confirmLabel="Create" onCancel={() => onClose(null)} />
      </form>
    </Dialog>
  );
}

/**
 * Modal dialog editing the document properties. The created and updated times are set when saving.
 *
 * @param props.properties Current properties.
 * @param props.onClose Receives the edited properties, or null when cancelled.
 * @returns The dialog.
 */
export function PropertiesModal({properties, onClose}: {properties: Properties; onClose: (result: Properties | null) => void}) {
  const [title, setTitle] = useState(properties.title ?? '');
  const [author, setAuthor] = useState(properties.author ?? '');
  const [tags, setTags] = useState((properties.tags ?? []).join(', '));

  return (
    <Dialog className="properties" onCancel={() => onClose(null)}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onClose({
            ...properties,
            title: title.trim() || undefined,
            author: author.trim() || undefined,
            tags: tags
              .split(',')
              .map((tag) => tag.trim())
              .filter(Boolean),
          });
        }}
      >
        <DialogTitle>Document Properties</DialogTitle>
        <TextField label="Title" value={title} onChange={setTitle} autoFocus />
        <TextField label="Author" value={author} onChange={setAuthor} />
        <TextField label="Created" value={properties.created ?? ''} />
        <TextField label="Updated" value={properties.updated ?? ''} />
        <TextField label="Tags, separated by commas" value={tags} onChange={setTags} />
        <Buttons confirmLabel="OK" onCancel={() => onClose(null)} />
      </form>
    </Dialog>
  );
}

/**
 * Modal dialog editing the editor settings.
 *
 * @param props.settings Current settings.
 * @param props.onClose Receives the edited settings, or null when cancelled.
 * @returns The dialog.
 */
export function SettingsModal({settings, onClose}: {settings: Settings; onClose: (result: Settings | null) => void}) {
  const [values, setValues] = useState(settings);
  const options: ['buttonTextLabels' | 'saveButtons' | 'gitControls', string][] = [
    ['buttonTextLabels', 'Use text for button labels'],
    ['saveButtons', 'Save and Revert buttons in the top bar'],
    ['gitControls', 'Git controls'],
  ];

  return (
    <Dialog onCancel={() => onClose(null)}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onClose(values);
        }}
      >
        <DialogTitle>Settings</DialogTitle>
        <Field className="field">
          <Label>Colour theme</Label>
          <Dropdown
            label="Colour theme"
            value={values.palette}
            options={palettes}
            onChange={(palette) => setValues((current) => ({...current, palette: palette as Palette}))}
          />
        </Field>
        <Field className="field">
          <Label>Appearance, shared with the pages of the server</Label>
          <Dropdown
            label="Appearance"
            value={values.appearance}
            options={appearances}
            onChange={(appearance) => setValues((current) => ({...current, appearance: appearance as Appearance}))}
          />
        </Field>
        <Field className="field">
          <Label>Document format</Label>
          <Dropdown
            label="Document format"
            value={values.format}
            options={formats}
            onChange={(format) => setValues((current) => ({...current, format: format as Format}))}
          />
        </Field>
        {options.map(([key, label], index) => (
          <Field key={key} className="setting">
            <Checkbox
              checked={values[key]}
              onChange={(checked) => setValues((current) => ({...current, [key]: checked}))}
              className="checkbox"
              autoFocus={index === 0}
            >
              <svg viewBox="0 0 14 14" fill="none">
                <path d="M3 8L6 11L11 3.5" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </Checkbox>
            <Label>{label}</Label>
          </Field>
        ))}
        <Buttons confirmLabel="OK" onCancel={() => onClose(null)} />
      </form>
    </Dialog>
  );
}

/**
 * Modal dialog committing the changed files.
 *
 * @param props.files Changed files committed together.
 * @param props.onClose Receives the commit message, or null when cancelled.
 * @returns The dialog.
 */
export function CommitModal({files, onClose}: {files: string[]; onClose: (result: string | null) => void}) {
  const [message, setMessage] = useState(files.length === 1 ? `Update ${files[0]}` : '');

  return (
    <Dialog className="commit" onCancel={() => onClose(null)}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (message.trim()) {
            onClose(message.trim());
          }
        }}
      >
        <DialogTitle>Commit</DialogTitle>
        <Description>
          {files.length === 1 ? 'The changed file is added and committed:' : `The ${files.length} changed files are added and committed:`}
        </Description>
        <ul className="commit-files">
          {files.map((file) => (
            <li key={file}>{file}</li>
          ))}
        </ul>
        <TextField label="Commit message" value={message} onChange={setMessage} autoFocus />
        <Buttons confirmLabel="Commit" onCancel={() => onClose(null)} />
      </form>
    </Dialog>
  );
}
