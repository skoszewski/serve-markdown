import {
  BlockTypeSelect,
  BoldItalicUnderlineToggles,
  ChangeCodeMirrorLanguage,
  codeBlockPlugin,
  codeMirrorPlugin,
  CodeToggle,
  ConditionalContents,
  CreateLink,
  DiffSourceToggleWrapper,
  diffSourcePlugin,
  headingsPlugin,
  imagePlugin,
  InsertCodeBlock,
  InsertImage,
  InsertTable,
  InsertThematicBreak,
  linkDialogPlugin,
  linkPlugin,
  listsPlugin,
  ListsToggle,
  markdownShortcutPlugin,
  MDXEditor,
  quotePlugin,
  Separator,
  tablePlugin,
  thematicBreakPlugin,
  toolbarPlugin,
  UndoRedo,
  type MDXEditorMethods,
} from '@mdxeditor/editor';
import {
  Checkbox,
  Field,
  Button as HeadlessButton,
  Label,
  Menu,
  MenuButton,
  MenuItem,
  MenuItems,
  MenuSeparator,
} from '@headlessui/react';
import {useCallback, useEffect, useMemo, useRef, useState, type ReactNode} from 'react';
import {Button, Dropdown, TextInput} from './controls';
import {Outline} from './Outline';
import {Resizer} from './Resizer';
import {
  Loading,
  Modal,
  Choice,
  CommitModal,
  PropertiesModal,
  SettingsModal,
  TemplateModal,
  type ModalRequest,
  type Properties,
  formats,
  type Format,
  type Settings,
  type Template,
} from './Modal';
import {rawHtmlPlugin} from './rawHtml';
import {appearances, codeHighlighting, palettes, type Appearance, type Palette} from './theme';

const codeBlockLanguages = {
  '': 'Plain text',
  bash: 'Bash',
  hcl: 'HCL',
  js: 'JavaScript',
  json: 'JSON',
  markdown: 'Markdown',
  mermaid: 'Mermaid',
  powershell: 'PowerShell',
  python: 'Python',
  sh: 'Shell (sh)',
  shell: 'Shell',
  terraform: 'Terraform',
  ts: 'TypeScript',
  yaml: 'YAML',
};

/**
 * Git state of the repository: the current branch, the branches, the changed editable files and the numbers of commits
 * not pushed and not pulled, null without an upstream branch.
 */
interface GitState {
  branch: string;
  branches: string[];
  changes: string[];
  ahead: number | null;
  behind: number | null;
}


const languages = {
  'en-US': 'English (US)',
  'en-GB': 'English (UK)',
  'pl-PL': 'Polski',
};
/**
 * Returns the route the server renders a document at.
 *
 * @param path Path of the document relative to the served directory.
 * @returns The URL path.
 */
function pageUrl(path: string): string {
  return `/${path.split('/').map(encodeURIComponent).join('/')}`;
}

/**
 * Resolves an image source of a document to the URL the server serves the image at.
 *
 * @param path Path of the document relative to the served directory.
 * @param source Image source in the Markdown.
 * @returns The image URL.
 */
function imageUrl(path: string, source: string): string {
  if (/^[a-z]+:/i.test(source) || source.startsWith('/')) {
    return source;
  }
  return new URL(source, `${location.origin}${pageUrl(path)}`).pathname;
}

/**
 * Returns the appearance the editor is drawn in, resolving the browser preference.
 *
 * @param appearance Chosen appearance.
 * @returns True for dark.
 */
function isDark(appearance: Appearance): boolean {
  return appearance === 'dark' || (appearance === 'auto' && matchMedia('(prefers-color-scheme: dark)').matches);
}

/**
 * The editor application: the file list and the visual Markdown editor of the selected file.
 *
 * @returns The application.
 */
export function App() {
  const editor = useRef<MDXEditorMethods>(null);
  const [files, setFiles] = useState<string[]>([]);
  const [filter, setFilter] = useState('');
  const [path, setPath] = useState<string>();
  const [original, setOriginal] = useState('');
  const [dirty, setDirty] = useState(false);
  const [status, setStatus] = useState('');
  const [revision, setRevision] = useState(0);
  const [spellCheck, setSpellCheck] = useState(localStorage.getItem('serve-markdown.editor.spellCheck') !== 'false');
  const [language, setLanguage] = useState(localStorage.getItem('serve-markdown.editor.language') ?? 'en-US');
  const [filesShown, setFilesShown] = useState(localStorage.getItem('serve-markdown.editor.filesShown') !== 'false');
  const [outlineShown, setOutlineShown] = useState(localStorage.getItem('serve-markdown.editor.outlineShown') !== 'false');
  const [filesWidth, setFilesWidth] = useState(Number(localStorage.getItem('serve-markdown.editor.filesWidth')) || 320);
  const [outlineWidth, setOutlineWidth] = useState(Number(localStorage.getItem('serve-markdown.editor.outlineWidth')) || 260);
  const [page, setPage] = useState<HTMLDivElement | null>(null);
  const [properties, setProperties] = useState<Properties>({});
  const [savedProperties, setSavedProperties] = useState<Properties>({});
  const [loading, setLoading] = useState<string>();
  const [dialog, setDialog] = useState<ReactNode>();
  const [spellCheckControls, setSpellCheckControls] = useState(false);
  const [buttonTextLabels, setButtonTextLabels] = useState(localStorage.getItem('serve-markdown.editor.buttonTextLabels') === 'true');
  const [gitControls, setGitControls] = useState(localStorage.getItem('serve-markdown.editor.gitControls') === 'true');
  const [saveButtons, setSaveButtons] = useState(localStorage.getItem('serve-markdown.editor.saveButtons') === 'true');
  const [git, setGit] = useState<GitState>();
  const [palette, setPalette] = useState<Palette>(() => {
    const stored = localStorage.getItem('serve-markdown.editor.palette');
    return stored && stored in palettes ? (stored as Palette) : 'github';
  });
  const [appearance, setAppearance] = useState<Appearance>(() => {
    const stored = localStorage.getItem('serve-markdown.theme');
    return stored === 'light' || stored === 'dark' ? stored : 'auto';
  });
  const [format, setFormat] = useState<Format>(() => {
    const stored = localStorage.getItem('serve-markdown.editor.format');
    return stored && stored in formats ? (stored as Format) : 'markdown';
  });

  // The theme follows the settings and, for the browser preference, the changes of the preference.
  useEffect(() => {
    localStorage.setItem('serve-markdown.editor.palette', palette);
    localStorage.setItem('serve-markdown.theme', appearance);
    const preference = matchMedia('(prefers-color-scheme: dark)');
    const update = () => {
      document.documentElement.dataset.palette = palette;
      document.documentElement.dataset.mode = isDark(appearance) ? 'dark' : 'light';
    };
    update();
    preference.addEventListener('change', update);
    return () => preference.removeEventListener('change', update);
  }, [palette, appearance]);

  useEffect(() => {
    localStorage.setItem('serve-markdown.editor.spellCheck', String(spellCheck));
    localStorage.setItem('serve-markdown.editor.language', language);
    localStorage.setItem('serve-markdown.editor.filesShown', String(filesShown));
    localStorage.setItem('serve-markdown.editor.outlineShown', String(outlineShown));
    localStorage.setItem('serve-markdown.editor.filesWidth', String(filesWidth));
    localStorage.setItem('serve-markdown.editor.outlineWidth', String(outlineWidth));
    localStorage.setItem('serve-markdown.editor.buttonTextLabels', String(buttonTextLabels));
    localStorage.setItem('serve-markdown.editor.gitControls', String(gitControls));
    localStorage.setItem('serve-markdown.editor.saveButtons', String(saveButtons));
    localStorage.setItem('serve-markdown.editor.format', format);
  }, [spellCheck, language, filesShown, outlineShown, filesWidth, outlineWidth, buttonTextLabels, gitControls, saveButtons, format]);

  const loadFiles = useCallback(
    () =>
      fetch('/_/editor/api/files')
        .then((response) => response.json() as Promise<string[]>)
        .then(setFiles)
        .catch((error: Error) => setStatus(`Cannot list files: ${error.message}`)),
    [],
  );

  useEffect(() => {
    void loadFiles();
    fetch('/_/editor/api/config')
      .then((response) => response.json() as Promise<{spellCheckControls: boolean}>)
      .then((config) => setSpellCheckControls(config.spellCheckControls))
      .catch((error: Error) => setStatus(`Cannot read the configuration: ${error.message}`));
  }, [loadFiles]);

  const preview = async () => {
    window.open(pageUrl(path!), 'preview');
  };

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => dirty && event.preventDefault();
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);

  const showLoading = async (file: string) => {
    setLoading(file);
    await new Promise((resolve) => requestAnimationFrame(() => setTimeout(resolve)));
  };

  useEffect(() => {
    if (loading) {
      requestAnimationFrame(() => setTimeout(() => setLoading(undefined)));
    }
  }, [revision]);

  const show = <T,>(render: (close: (result: T | null) => void) => ReactNode) =>
    new Promise<T | null>((resolve) =>
      setDialog(
        render((result) => {
          setDialog(undefined);
          resolve(result);
        }),
      ),
    );

  const ask = (request: ModalRequest) => show<string>((close) => <Modal request={request} onClose={close} />);

  // Asks to save or revert the unsaved changes before leaving the open file, returns false when cancelled.
  const confirmDiscard = async () => {
    if (!dirty) {
      return true;
    }
    const choice = await show<string>((close) => (
      <Choice
        title="Unsaved changes"
        message={`${path} has unsaved changes.`}
        choices={{save: 'Save', revert: 'Revert'}}
        onClose={close}
      />
    ));
    return choice === 'revert' || (choice === 'save' && (await save()));
  };

  const open = async (file: string, confirmed = false) => {
    if (!confirmed && !(await confirmDiscard())) {
      return;
    }
    await showLoading(file);
    const response = await fetch(`/_/editor/api/file?path=${encodeURIComponent(file)}`);
    if (!response.ok) {
      setLoading(undefined);
      setStatus(await response.text());
      return;
    }
    const document = (await response.json()) as {properties: Properties; body: string};
    setOriginal(document.body);
    setProperties(document.properties ?? {});
    setSavedProperties(document.properties ?? {});
    setPath(file);
    setRevision((current) => current + 1);
    setDirty(false);
    setStatus('');
  };

  const save = useCallback(async (): Promise<boolean> => {
    if (!path || !editor.current) {
      return false;
    }
    const markdown = editor.current.getMarkdown();
    const response = await fetch(`/_/editor/api/file?path=${encodeURIComponent(path)}`, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({properties, body: markdown}),
    });
    if (!response.ok) {
      setStatus(`Save failed: ${await response.text()}`);
      return false;
    }
    const saved = (await response.json()) as Properties;
    setOriginal(markdown);
    setProperties(saved);
    setSavedProperties(saved);
    setDirty(false);
    setStatus(`Saved ${new Date().toLocaleTimeString()}`);
    return true;
  }, [path, properties]);

  const createFile = async (file: string, template?: string) => {
    const response = await fetch(`/_/editor/api/file?path=${encodeURIComponent(file)}`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({template}),
    });
    if (!response.ok) {
      setStatus(await response.text());
      return;
    }
    await loadFiles();
    await open(file, true);
  };

  const newFileDirectory = (path ?? files[0] ?? '').replace(/[^/]+$/, '');

  const create = async () => {
    if (!(await confirmDiscard())) {
      return;
    }
    const file = await ask({
      title: 'New file',
      message: 'Path of the new file, ending with .md or .markdown:',
      value: newFileDirectory,
      confirmLabel: 'Create',
    });
    if (file) {
      await createFile(file);
    }
  };

  const createFolder = async () => {
    if (!(await confirmDiscard())) {
      return;
    }
    const folder = await ask({
      title: 'New folder',
      message: 'Path of the new folder. The folder gets an index.md page:',
      value: newFileDirectory,
      confirmLabel: 'Create',
    });
    if (folder) {
      await createFile(`${folder.replace(/\/+$/, '')}/index.md`);
    }
  };

  const createFromTemplate = async () => {
    if (!(await confirmDiscard())) {
      return;
    }
    const response = await fetch('/_/editor/api/templates');
    if (!response.ok) {
      setStatus(`Cannot list templates: ${await response.text()}`);
      return;
    }
    const templates = (await response.json()) as Template[];
    const result = await show<{template: string; path: string}>((close) => (
      <TemplateModal templates={templates} path={newFileDirectory} onClose={close} />
    ));
    if (result) {
      await createFile(result.path, result.template);
    }
  };

  const editProperties = async () => {
    const result = await show<Properties>((close) => <PropertiesModal properties={properties} onClose={close} />);
    if (result && JSON.stringify(result) !== JSON.stringify(properties)) {
      setProperties(result);
      setDirty(true);
    }
  };

  const editSettings = async () => {
    const result = await show<Settings>((close) => (
      <SettingsModal settings={{buttonTextLabels, saveButtons, gitControls, palette, appearance, format}} onClose={close} />
    ));
    if (result) {
      setButtonTextLabels(result.buttonTextLabels);
      setSaveButtons(result.saveButtons);
      setGitControls(result.gitControls);
      setPalette(result.palette);
      setAppearance(result.appearance);
      // The open document is read again in the new format, so its unsaved changes are saved or reverted first.
      if (result.format !== format && (await confirmDiscard())) {
        setFormat(result.format);
        setProperties(savedProperties);
        setDirty(false);
      }
    }
  };

  const loadGit = useCallback(async () => {
    const response = await fetch('/_/editor/api/git');
    if (!response.ok) {
      setStatus(`Cannot read the Git state: ${await response.text()}`);
      return;
    }
    setGit((await response.json()) as GitState);
  }, []);

  // The Git state follows the saved and created files.
  useEffect(() => {
    if (gitControls) {
      void loadGit();
    }
  }, [gitControls, loadGit, files, savedProperties]);

  const commit = async () => {
    if (!git || git.changes.length === 0) {
      return;
    }
    if (
      dirty &&
      (await ask({
        title: 'Unsaved changes',
        message: `The unsaved changes of ${path} are not committed, only the saved files are.`,
        confirmLabel: 'Continue',
      })) === null
    ) {
      return;
    }
    const message = await show<string>((close) => <CommitModal files={git.changes} onClose={close} />);
    if (!message) {
      return;
    }
    const response = await fetch('/_/editor/api/git/commit', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({message}),
    });
    setStatus(response.ok ? `Committed ${((await response.json()) as string[]).length} file(s)` : `Commit failed: ${await response.text()}`);
    await loadGit();
  };

  const push = async () => {
    setStatus('Pushing...');
    const response = await fetch('/_/editor/api/git/push', {method: 'POST'});
    setStatus(response.ok ? `Pushed ${git?.branch}` : `Push failed: ${await response.text()}`);
    await loadGit();
  };

  const fetchRemotes = async () => {
    setStatus('Fetching...');
    const response = await fetch('/_/editor/api/git/fetch', {method: 'POST'});
    setStatus(response.ok ? 'Fetched' : `Fetch failed: ${await response.text()}`);
    await loadGit();
  };

  const pull = async () => {
    if (!(await confirmDiscard())) {
      return;
    }
    setStatus('Pulling...');
    const response = await fetch('/_/editor/api/git/pull', {method: 'POST'});
    if (!response.ok) {
      setStatus(`Pull failed: ${await response.text()}`);
      return;
    }
    await reloadRepository();
    setStatus(`Pulled ${git?.branch}`);
  };

  const switchBranch = async (branch: string) => {
    if (branch === git?.branch || !(await confirmDiscard())) {
      return;
    }
    const response = await fetch('/_/editor/api/git/switch', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({branch}),
    });
    if (!response.ok) {
      setStatus(`Switching to ${branch} failed: ${await response.text()}`);
      return;
    }
    await reloadRepository();
    setStatus(`Switched to ${branch}`);
  };

  // Reloads the file list, the open page and the Git state after the working tree changed.
  const reloadRepository = async () => {
    const branchFiles = (await (await fetch('/_/editor/api/files')).json()) as string[];
    setFiles(branchFiles);
    if (path && branchFiles.includes(path)) {
      await open(path, true);
    } else {
      setPath(undefined);
      setDirty(false);
    }
    await loadGit();
  };

  const revert = async () => {
    if (
      !dirty ||
      (await ask({title: 'Revert', message: `Revert the unsaved changes of ${path}?`, confirmLabel: 'Revert'})) === null
    ) {
      return;
    }
    await showLoading(path!);
    setProperties(savedProperties);
    setRevision((current) => current + 1);
    setDirty(false);
    setStatus('Reverted');
  };

  // Ctrl+S or Cmd+S saves, handled in the capture phase before the editor and the code blocks get the key.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 's') {
        event.preventDefault();
        event.stopPropagation();
        if (dirty) {
          void save();
        }
      }
    };
    window.addEventListener('keydown', onKeyDown, {capture: true});
    return () => window.removeEventListener('keydown', onKeyDown, {capture: true});
  }, [save, dirty]);

  const plugins = useMemo(
    () => [
      headingsPlugin(),
      listsPlugin(),
      quotePlugin(),
      thematicBreakPlugin(),
      linkPlugin(),
      linkDialogPlugin(),
      imagePlugin({imagePreviewHandler: async (source) => imageUrl(path ?? '', source)}),
      tablePlugin(),
      codeBlockPlugin({defaultCodeBlockLanguage: ''}),
      codeMirrorPlugin({codeBlockLanguages, codeMirrorExtensions: [codeHighlighting]}),
      rawHtmlPlugin(),
      diffSourcePlugin({viewMode: 'rich-text', diffMarkdown: original}),
      markdownShortcutPlugin(),
      toolbarPlugin({
        toolbarContents: () => (
          <DiffSourceToggleWrapper>
            <ConditionalContents
              options={[
                {when: (focused) => focused?.editorType === 'codeblock', contents: () => <ChangeCodeMirrorLanguage />},
                {
                  fallback: () => (
                    <>
                      <UndoRedo />
                      <Separator />
                      <BlockTypeSelect />
                      <BoldItalicUnderlineToggles />
                      <CodeToggle />
                      <Separator />
                      <ListsToggle />
                      <CreateLink />
                      <InsertImage />
                      <InsertTable />
                      <InsertThematicBreak />
                      <Separator />
                      <InsertCodeBlock />
                    </>
                  ),
                },
              ]}
            />
          </DiffSourceToggleWrapper>
        ),
      }),
    ],
    [path, original],
  );

  const shownFiles = files.filter((file) => file.toLowerCase().includes(filter.toLowerCase()));

  return (
    <div
      className="layout"
      style={{
        // The side panels give up their width before the editor gets narrower than its top bar.
        gridTemplateColumns: [
          filesShown && `minmax(0, ${filesWidth}px) 5px`,
          `minmax(${(buttonTextLabels ? 560 : 370) + (saveButtons ? (buttonTextLabels ? 170 : 100) : 0) + (spellCheckControls ? 240 : 0)}px, 1fr)`,
          outlineShown && `5px minmax(0, ${outlineWidth}px)`,
        ]
          .filter(Boolean)
          .join(' '),
      }}
    >
      <aside className="files" hidden={!filesShown}>
        <Menu>
          <MenuButton className="button button-primary new-menu">
            File{dirty ? ' *' : ''}
            <svg viewBox="0 0 12 12" fill="none" aria-hidden="true">
              <path d="M3 4.5L6 7.5L9 4.5" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </MenuButton>
          <MenuItems anchor="bottom start" className="menu-items">
            <MenuItem>
              <button className="menu-item" onClick={() => void create()}>
                New file
              </button>
            </MenuItem>
            <MenuItem>
              <button className="menu-item" onClick={() => void createFromTemplate()}>
                New file from Template
              </button>
            </MenuItem>
            <MenuItem>
              <button className="menu-item" onClick={() => void createFolder()}>
                New Folder
              </button>
            </MenuItem>
            <MenuSeparator className="menu-separator" />
            <MenuItem disabled={!path || !dirty}>
              <button className="menu-item" onClick={() => void save()}>
                Save
                <kbd>Ctrl+S</kbd>
              </button>
            </MenuItem>
            <MenuItem disabled={!path || !dirty}>
              <button className="menu-item" onClick={() => void revert()}>
                Revert
              </button>
            </MenuItem>
          </MenuItems>
        </Menu>
        {gitControls && git && (
          <div className="git">
            <Dropdown
              label="Branch"
              value={git.branch}
              options={Object.fromEntries((git.branch ? [git.branch, ...git.branches] : git.branches).map((branch) => [branch, branch]))}
              onChange={(branch) => void switchBranch(branch)}
            />
            <div className="git-buttons">
              <Button variant="secondary" onClick={() => void fetchRemotes()} title="Fetch the branches and commits of the remotes">
                Fetch
              </Button>
              <Button
                variant="secondary"
                onClick={() => void pull()}
                disabled={git.behind === 0 || git.behind === null}
                title={git.behind === null ? 'The branch has no upstream branch' : `${git.behind} commit(s) to pull`}
              >
                Pull{git.behind ? ` (${git.behind})` : ''}
              </Button>
              <Button
                variant="secondary"
                onClick={() => void commit()}
                disabled={git.changes.length === 0}
                title={git.changes.length ? git.changes.join('\n') : 'No changed files'}
              >
                Commit{git.changes.length ? ` (${git.changes.length})` : ''}
              </Button>
              <Button
                variant="secondary"
                onClick={() => void push()}
                disabled={git.ahead === 0 || !git.branch}
                title={git.ahead === null ? 'Push and set the upstream branch' : `${git.ahead} commit(s) to push`}
              >
                Push{git.ahead ? ` (${git.ahead})` : ''}
              </Button>
            </div>
          </div>
        )}
        <TextInput
          className="filter"
          placeholder="Filter files"
          aria-label="Filter files"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
        <ul>
          {shownFiles.map((file) => (
            <li key={file}>
              <HeadlessButton className={file === path ? 'selected' : undefined} onClick={() => void open(file)} title={file}>
                {file}
              </HeadlessButton>
            </li>
          ))}
        </ul>
      </aside>
      {filesShown && <Resizer side="left" width={filesWidth} onChange={setFilesWidth} label="Resize the file list" />}
      <main className="editor">
        <header className="bar">
          <HeadlessButton
            className="panel-toggle"
            onClick={() => setFilesShown((shown) => !shown)}
            aria-label={filesShown ? 'Hide the file list' : 'Show the file list'}
            title={filesShown ? 'Hide the file list' : 'Show the file list'}
          >
            <svg viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <rect x="1.5" y="2.5" width="13" height="11" rx="1.5" strokeWidth={1.5} />
              <path d="M6 2.5V13.5" strokeWidth={1.5} />
              {filesShown ? (
                <path d="M11 6L9 8L11 10" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
              ) : (
                <path d="M9 6L11 8L9 10" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
              )}
            </svg>
          </HeadlessButton>
          <span className="path">
            {path ?? 'Select a file'}
            {dirty ? ' *' : ''}
          </span>
          {path && properties.title && <span className="title">{properties.title}</span>}
          <span className="status">{status}</span>
          {spellCheckControls && (
            <>
              <Field className="spell-check">
                <Checkbox checked={spellCheck} onChange={setSpellCheck} className="checkbox">
                  <svg viewBox="0 0 14 14" fill="none">
                    <path d="M3 8L6 11L11 3.5" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                </Checkbox>
                <Label>Spell check</Label>
              </Field>
              <Dropdown
                surface="bar"
                label="Spell check language"
                value={language}
                options={languages}
                onChange={setLanguage}
                disabled={!spellCheck}
              />
            </>
          )}
          {[
            {
              label: 'Properties',
              title: 'Document Properties',
              onClick: editProperties,
              disabled: !path,
              icon: (
                <>
                  <path d="M3.5 1.5H9.5L12.5 4.5V14.5H3.5Z" strokeWidth={1.5} strokeLinejoin="round" />
                  <path d="M9.5 1.5V4.5H12.5M6 8H10M6 11H10" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
                </>
              ),
            },
            ...(saveButtons
              ? [
                  {
                    label: 'Save',
                    title: 'Save (Ctrl+S)',
                    primary: true,
                    onClick: save,
                    disabled: !path || !dirty,
                    icon: (
                      <>
                        <path d="M2.5 2.5H11.5L13.5 4.5V13.5H2.5Z" strokeWidth={1.5} strokeLinejoin="round" />
                        <path d="M5 2.5V6H10.5V2.5M5 13.5V9.5H11V13.5" strokeWidth={1.5} strokeLinejoin="round" />
                      </>
                    ),
                  },
                  {
                    label: 'Revert',
                    title: 'Revert the unsaved changes',
                    onClick: revert,
                    disabled: !path || !dirty,
                    icon: (
                      <>
                        <path d="M2.5 2.5V6.5H6.5" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
                        <path d="M3 6.5A5.5 5.5 0 1 1 3.5 11" strokeWidth={1.5} strokeLinecap="round" />
                      </>
                    ),
                  },
                ]
              : []),
            {
              label: 'Preview',
              title: dirty ? 'Preview the saved file, without the unsaved changes' : 'Preview',
              onClick: preview,
              disabled: !path,
              icon: (
                <>
                  <path
                    d="M1.5 8C3 5.2 5.3 3.5 8 3.5S13 5.2 14.5 8C13 10.8 10.7 12.5 8 12.5S3 10.8 1.5 8Z"
                    strokeWidth={1.5}
                    strokeLinejoin="round"
                  />
                  <circle cx="8" cy="8" r="2" strokeWidth={1.5} />
                </>
              ),
            },
            {
              label: 'Settings',
              title: 'Settings',
              onClick: editSettings,
              disabled: false,
              icon: (
                <>
                  <circle cx="8" cy="8" r="4.5" strokeWidth={1.5} />
                  <path
                    d="M13 8H14.5M11.5 11.5L12.6 12.6M8 13V14.5M4.5 11.5L3.4 12.6M3 8H1.5M4.5 4.5L3.4 3.4M8 3V1.5M11.5 4.5L12.6 3.4"
                    strokeWidth={2}
                    strokeLinecap="round"
                  />
                </>
              ),
            },
            {
              label: 'Theme',
              title: `Theme: ${appearances[appearance]}`,
              onClick: async () => {
                // The appearance goes from the browser's preference to light, to dark and back, as on the pages.
                const order = Object.keys(appearances) as Appearance[];
                setAppearance(order[(order.indexOf(appearance) + 1) % order.length]);
              },
              disabled: false,
              icon: {
                auto: (
                  <>
                    <circle cx="8" cy="8" r="6" strokeWidth={1.5} />
                    <path d="M8 2a6 6 0 0 1 0 12z" fill="currentColor" />
                  </>
                ),
                light: (
                  <>
                    <circle cx="8" cy="8" r="3" strokeWidth={1.5} />
                    <path
                      d="M8 1v2M8 13v2M1 8h2M13 8h2M3.05 3.05l1.4 1.4M11.55 11.55l1.4 1.4M3.05 12.95l1.4-1.4M11.55 4.45l1.4-1.4"
                      strokeWidth={1.5}
                      strokeLinecap="round"
                    />
                  </>
                ),
                dark: <path d="M13.5 10A6 6 0 0 1 6 2.5a6 6 0 1 0 7.5 7.5z" strokeWidth={1.5} strokeLinejoin="round" />,
              }[appearance],
            },
          ].map((action: {label: string; title: string; primary?: boolean; onClick: () => Promise<unknown>; disabled: boolean; icon: ReactNode}) => (
            <Button
              key={action.label}
              variant={action.primary ? 'bar-primary' : 'bar'}
              className={buttonTextLabels ? undefined : 'button-icon'}
              onClick={() => void action.onClick()}
              disabled={action.disabled}
              aria-label={buttonTextLabels ? undefined : action.label}
              title={action.title}
            >
              {!buttonTextLabels ? (
                <svg viewBox="0 0 16 16" fill="none" aria-hidden="true">
                  {action.icon}
                </svg>
              ) : (
                action.label
              )}
            </Button>
          ))}
          <HeadlessButton
            className="panel-toggle"
            onClick={() => setOutlineShown((shown) => !shown)}
            aria-label={outlineShown ? 'Hide the outline' : 'Show the outline'}
            title={outlineShown ? 'Hide the outline' : 'Show the outline'}
          >
            <svg viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <rect x="1.5" y="2.5" width="13" height="11" rx="1.5" strokeWidth={1.5} />
              <path d="M10 2.5V13.5" strokeWidth={1.5} />
              {outlineShown ? (
                <path d="M5 6L7 8L5 10" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
              ) : (
                <path d="M7 6L5 8L7 10" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" />
              )}
            </svg>
          </HeadlessButton>
        </header>
        {path && (
          <div className="page" lang={spellCheckControls ? language : undefined} ref={setPage}>
            <MDXEditor
              key={`${path}:${revision}:${format}`}
              ref={editor}
              markdown={original}
              plugins={plugins}
              suppressHtmlProcessing={format === 'markdown'}
              className="mdx"
              contentEditableClassName="content"
              spellCheck={!spellCheckControls || spellCheck}
              onChange={(_markdown, initialMarkdownNormalize) => !initialMarkdownNormalize && setDirty(true)}
              onError={({error}) => setStatus(`Parse error: ${error}. Switch to source mode to edit the file.`)}
            />
          </div>
        )}
      </main>
      {outlineShown && <Resizer side="right" width={outlineWidth} onChange={setOutlineWidth} label="Resize the outline" />}
      {outlineShown && <Outline root={page} />}
      {loading && <Loading message={`Loading ${loading} ...`} />}
      {dialog}
    </div>
  );
}
