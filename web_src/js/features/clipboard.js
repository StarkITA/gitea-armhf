import {showTemporaryTooltip} from '../modules/tippy.js';
import {toAbsoluteUrl} from '../utils.js';

const {copy_success, copy_error} = window.config.i18n;

export async function copyToClipboard(content) {
  if (content instanceof Blob) {
    const item = new ClipboardItem({[content.type]: content});
    await navigator.clipboard.write([item]);
  } else { // text
    try {
      await navigator.clipboard.writeText(content);
    } catch {
      return fallbackCopyToClipboard(content);
    }
  }
  return true;
}

// Fallback to use if navigator.clipboard doesn't exist. Achieved via creating
// a temporary textarea element, selecting the text, and using document.execCommand
function fallbackCopyToClipboard(text) {
  if (!document.execCommand) return false;

  const tempTextArea = document.createElement('textarea');
  tempTextArea.value = text;

  // avoid scrolling
  tempTextArea.style.top = 0;
  tempTextArea.style.left = 0;
  tempTextArea.style.position = 'fixed';

  document.body.appendChild(tempTextArea);

  tempTextArea.select();

  // if unsecure (not https), there is no navigator.clipboard, but we can still
  // use document.execCommand to copy to clipboard
  const success = document.execCommand('copy');

  document.body.removeChild(tempTextArea);

  return success;
}

// For all DOM elements with [data-clipboard-target] or [data-clipboard-text],
// this copy-to-clipboard will work for them
export function initGlobalCopyToClipboardListener() {
  document.addEventListener('click', (e) => {
    let target = e.target;
    // in case <button data-clipboard-text><svg></button>, so we just search
    // up to 3 levels for performance
    for (let i = 0; i < 3 && target; i++) {
      let txt = target.getAttribute('data-clipboard-text');
      if (txt && target.getAttribute('data-clipboard-text-type') === 'url') {
        txt = toAbsoluteUrl(txt);
      }
      const text = txt || document.querySelector(target.getAttribute('data-clipboard-target'))?.value;

      if (text) {
        e.preventDefault();

        (async() => {
          const success = await copyToClipboard(text);
          showTemporaryTooltip(target, success ? copy_success : copy_error);
        })();

        break;
      }
      target = target.parentElement;
    }
  });
}
