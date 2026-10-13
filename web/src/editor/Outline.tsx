import {Button} from '@headlessui/react';
import {useEffect, useState} from 'react';

/**
 * Outline of the edited document: the headings of the editor content, following the edits, scrolling to the
 * clicked heading.
 *
 * @param props.root Element containing the editor, null without an open file.
 * @returns The outline panel.
 */
export function Outline({root}: {root: HTMLElement | null}) {
  const [headings, setHeadings] = useState<HTMLElement[]>([]);

  useEffect(() => {
    if (!root) {
      setHeadings([]);
      return;
    }
    let frame = 0;
    const update = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() =>
        setHeadings([...root.querySelectorAll<HTMLElement>('[contenteditable="true"] :is(h1, h2, h3, h4, h5, h6)')]),
      );
    };
    update();
    const observer = new MutationObserver(update);
    observer.observe(root, {childList: true, subtree: true, characterData: true});
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [root]);

  return (
    <aside className="outline">
      <h2>Outline</h2>
      {headings.length === 0 ? (
        <p className="outline-empty">No headings</p>
      ) : (
        <ul>
          {headings.map((heading, index) => (
            <li key={index}>
              <Button
                className={`outline-item outline-${heading.tagName.toLowerCase()}`}
                onClick={() => heading.scrollIntoView({block: 'start', behavior: 'smooth'})}
                title={heading.textContent ?? ''}
              >
                {heading.textContent || '(empty heading)'}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </aside>
  );
}
