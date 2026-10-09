// The highlight.js the page highlights code with: its common languages and the ones registered
// below.

import hljs from "highlight.js/lib/common";
import powershell from "highlight.js/lib/languages/powershell";

hljs.registerLanguage("powershell", powershell);

export default hljs;
