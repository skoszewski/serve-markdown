// The page shell: it polls the server for the document this route addresses and renders it,
// reading its settings from the pageConfig the server wrote into the page.

const content = document.getElementById("_content");
const outline = document.getElementById("_outline");
const documentCSS = document.getElementById("_document-css");

let lastMtime = null;
let mermaidScript = null;

/**
 * Loads the mermaid bundle, once, the first time a document holds a diagram.
 *
 * @returns {Promise<void>} resolved once the bundle has run.
 */
function loadMermaid() {
  if (mermaidScript === null) {
    mermaidScript = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = pageConfig.mermaidJS;
      script.onload = () => resolve();
      script.onerror = () => reject(new Error("mermaid could not be loaded from " + pageConfig.mermaidJS));
      document.head.appendChild(script);
    });
  }
  return mermaidScript;
}

/**
 * Renders every fenced mermaid block of the document as a diagram.
 *
 * A block that mermaid cannot draw keeps its source visible as a code block.
 *
 * @returns {Promise<void>} resolved once the diagrams have been drawn.
 */
async function renderDiagrams() {
  const blocks = content.querySelectorAll("pre > code.language-mermaid");
  if (!pageConfig.mermaidJS || blocks.length === 0) {
    return;
  }

  const nodes = [];
  blocks.forEach((block) => {
    const node = document.createElement("div");
    node.className = "mermaid";
    node.textContent = block.textContent;
    block.parentElement.replaceWith(node);
    nodes.push(node);
  });

  try {
    await loadMermaid();
    const dark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    mermaid.initialize({ startOnLoad: false, theme: dark ? "dark" : "default" });
    await mermaid.run({ nodes });
  } catch (error) {
    nodes.forEach((node) => {
      if (node.querySelector("svg")) {
        return;
      }
      const block = document.createElement("code");
      block.className = "language-mermaid";
      block.textContent = node.textContent;
      const wrapper = document.createElement("pre");
      wrapper.appendChild(block);
      node.replaceWith(wrapper);
    });
  }
}

/**
 * Returns the anchor name for a heading's text, kept unique within the document.
 *
 * @param {string} text the heading's text.
 * @param {Set<string>} taken the names already given out.
 * @returns {string} the name to link the heading by.
 */
function headingSlug(text, taken) {
  const base = text.toLowerCase().trim().replace(/[^\p{L}\p{N}]+/gu, "-").replace(/^-+|-+$/g, "") || "section";
  let slug = base;
  for (let suffix = 2; taken.has(slug); suffix += 1) {
    slug = base + "-" + suffix;
  }
  taken.add(slug);
  return slug;
}

/**
 * Rebuilds the outline from the headings of the rendered document.
 *
 * The lists follow the heading levels, a level that skips a step opening the list between
 * them. The numbering is the outline's styling, not this function's doing.
 */
function buildOutline() {
  if (outline === null) {
    return;
  }

  const root = document.createElement("ol");
  const taken = new Set();
  const lists = [];

  content.querySelectorAll("h1, h2, h3, h4, h5, h6").forEach((heading) => {
    const level = Number(heading.tagName.substring(1));
    if (lists.length === 0) {
      lists.push({ level: level, list: root });
    }
    while (lists.length > 1 && level < lists[lists.length - 1].level) {
      lists.pop();
    }
    while (level > lists[lists.length - 1].level) {
      const parent = lists[lists.length - 1];
      const holder = parent.list.lastElementChild || parent.list.appendChild(document.createElement("li"));
      const nested = document.createElement("ol");
      holder.appendChild(nested);
      lists.push({ level: parent.level + 1, list: nested });
    }

    heading.id = headingSlug(heading.textContent, taken);
    const link = document.createElement("a");
    link.href = "#" + heading.id;
    link.textContent = heading.textContent;
    const item = document.createElement("li");
    item.appendChild(link);
    lists[lists.length - 1].list.appendChild(item);
  });

  outline.textContent = "";
  if (root.childElementCount > 0) {
    outline.appendChild(root);
  }
}

/**
 * Gives every link into this server the query the page was opened with, so that browsing from
 * the document keeps its settings.
 *
 * A link that carries a query of its own, or points at another host or at this page, is left
 * as it is.
 */
function carryQuery() {
  if (!location.search) {
    return;
  }
  content.querySelectorAll("a[href]").forEach((link) => {
    const href = link.getAttribute("href");
    if (href.startsWith("#")) {
      return;
    }
    const target = new URL(href, location.href);
    if (target.origin !== location.origin || target.search) {
      return;
    }
    link.setAttribute("href", target.pathname + location.search + target.hash);
  });
}

/**
 * Renders one version of the document, with the CSS its front matter asks for.
 *
 * @param {string} text the document, as Markdown.
 * @param {string} css the CSS to style the page with.
 */
function render(text, css) {
  documentCSS.textContent = css || "";
  if (!window.marked || !window.DOMPurify) {
    const pre = document.createElement("pre");
    pre.textContent = text;
    content.innerHTML = "";
    content.appendChild(pre);
    return;
  }

  content.innerHTML = DOMPurify.sanitize(marked.parse(text, { gfm: true }));
  if (window.hljs) {
    content.querySelectorAll("pre code:not(.language-mermaid)").forEach((block) => hljs.highlightElement(block));
  }
  buildOutline();
  carryQuery();
  renderDiagrams();
}

/**
 * Asks the server for the document and renders it when it differs from the one on the page.
 *
 * @returns {Promise<void>} resolved once the answer has been handled.
 */
async function poll() {
  let data;
  try {
    data = await (await fetch("/content" + pageConfig.contentQuery)).json();
  } catch (error) {
    return;
  }
  if (data.mtime !== null && data.mtime !== lastMtime) {
    lastMtime = data.mtime;
    render(data.text, data.css);
  } else if (data.mtime === null) {
    render("(file not found: " + data.error + ")", "");
  }
}

poll();
setInterval(poll, pageConfig.watchIntervalMS);
