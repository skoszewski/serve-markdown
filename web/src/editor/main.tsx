import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';
import '@mdxeditor/editor/style.css';
import './editor.css';
import {App} from './App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
