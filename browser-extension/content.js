// content.js — CrabClaw Content Script
// Injected into all web pages. Provides DOM reading, element querying,
// accessibility tree extraction, and screenshot coordination.
// Communicates with background.js via chrome.runtime messaging.

'use strict';

// ---- Guard: skip if already injected ----
if (window.__crabclaw_content_injected) {
  // Duplicate injection (e.g., manual reload) — do nothing.
} else {
  window.__crabclaw_content_injected = true;

  // ---- Message Handler ----
  chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
    if (!msg || !msg.action) return false;

    switch (msg.action) {
      case 'cs_ping':
        sendResponse({ ok: true, url: location.href });
        return false;

      case 'cs_get_text':
        sendResponse(getPageText(msg));
        return false;

      case 'cs_get_html':
        sendResponse(getPageHTML(msg));
        return false;

      case 'cs_query':
        sendResponse(querySelector(msg));
        return false;

      case 'cs_query_all':
        sendResponse(querySelectorAll(msg));
        return false;

      case 'cs_accessibility_tree':
        sendResponse(getAccessibilityTree(msg));
        return false;

      // captureScreenshot is handled by background.js (chrome.tabs.captureVisibleTab)
      // — content script just confirms readiness.
      case 'cs_screenshot_ready':
        sendResponse({ ok: true });
        return false;

      // S2: Enhanced interaction commands.
      case 'cs_click':
        sendResponse(clickElement(msg));
        return false;

      case 'cs_fill':
        sendResponse(fillForm(msg));
        return false;

      case 'cs_scroll':
        sendResponse(scrollTo(msg));
        return false;

      case 'cs_highlight':
        sendResponse(highlightElement(msg));
        return false;

      case 'cs_annotate_som':
        sendResponse(annotateSOM(msg));
        return false;

      case 'cs_clear_annotations':
        sendResponse(clearAnnotations());
        return false;

      case 'cs_upload_file':
        handleUploadFile(msg, sendResponse);
        return true; // async

      default:
        return false;
    }
  });

  // ---- S1-3: getPageText ----
  function getPageText(msg) {
    try {
      const selector = msg.selector || 'body';
      const el = document.querySelector(selector);
      if (!el) {
        return { error: `Element not found: ${selector}` };
      }

      let text = el.innerText || el.textContent || '';

      // Clean up excessive whitespace.
      text = text.replace(/\n{3,}/g, '\n\n').trim();

      // Truncate if too large (default 500KB).
      const maxLen = msg.maxLength || 512000;
      const truncated = text.length > maxLen;
      if (truncated) {
        text = text.slice(0, maxLen);
      }

      return {
        text,
        length: text.length,
        truncated,
        url: location.href,
        title: document.title,
      };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S1-4: getPageHTML ----
  function getPageHTML(msg) {
    try {
      const selector = msg.selector || 'html';
      const el = document.querySelector(selector);
      if (!el) {
        return { error: `Element not found: ${selector}` };
      }

      const outer = msg.outer !== false; // default: outerHTML
      let html = outer ? el.outerHTML : el.innerHTML;

      // Truncate if too large (default 1MB).
      const maxLen = msg.maxLength || 1048576;
      const truncated = html.length > maxLen;
      if (truncated) {
        html = html.slice(0, maxLen);
      }

      return {
        html,
        length: html.length,
        truncated,
        url: location.href,
        title: document.title,
      };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S1-5: querySelector ----
  function querySelector(msg) {
    try {
      const selector = msg.selector;
      if (!selector) {
        return { error: 'Missing selector' };
      }

      const el = document.querySelector(selector);
      if (!el) {
        return { found: false, selector };
      }

      return {
        found: true,
        selector,
        element: describeElement(el),
      };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S1-6: querySelectorAll ----
  function querySelectorAll(msg) {
    try {
      const selector = msg.selector;
      if (!selector) {
        return { error: 'Missing selector' };
      }

      const maxResults = msg.maxResults || 100;
      const els = document.querySelectorAll(selector);
      const elements = [];
      const total = els.length;

      for (let i = 0; i < Math.min(total, maxResults); i++) {
        elements.push(describeElement(els[i]));
      }

      return {
        selector,
        total,
        returned: elements.length,
        truncated: total > maxResults,
        elements,
      };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S1-7: getAccessibilityTree ----
  function getAccessibilityTree(msg) {
    try {
      const root = msg.selector
        ? document.querySelector(msg.selector)
        : document.body;

      if (!root) {
        return { error: 'Root element not found' };
      }

      const maxDepth = msg.maxDepth || 10;
      const maxNodes = msg.maxNodes || 500;
      let nodeCount = 0;

      function buildNode(el, depth) {
        if (depth > maxDepth || nodeCount >= maxNodes) return null;

        // Skip invisible elements.
        if (el.offsetParent === null && el !== document.body && el.tagName !== 'HTML') {
          const style = getComputedStyle(el);
          if (style.display === 'none' || style.visibility === 'hidden') return null;
        }

        // Determine ARIA role.
        const role = el.getAttribute('role') ||
          inferRole(el) ||
          '';

        // Determine accessible name.
        const name = el.getAttribute('aria-label') ||
          el.getAttribute('alt') ||
          el.getAttribute('title') ||
          el.getAttribute('placeholder') ||
          (isInteractive(el) ? (el.textContent || '').trim().slice(0, 100) : '') ||
          '';

        nodeCount++;

        const node = {
          ref: nodeCount,
          tag: el.tagName.toLowerCase(),
          role: role || undefined,
          name: name || undefined,
        };

        // Add value for form elements.
        if (el.value !== undefined && el.value !== '') {
          node.value = String(el.value).slice(0, 200);
        }

        // Add checked/selected state.
        if (el.checked !== undefined) node.checked = el.checked;
        if (el.selected !== undefined) node.selected = el.selected;
        if (el.disabled) node.disabled = true;

        // Add bounding rect for interactive elements.
        if (isInteractive(el)) {
          const rect = el.getBoundingClientRect();
          node.bounds = {
            x: Math.round(rect.x),
            y: Math.round(rect.y),
            w: Math.round(rect.width),
            h: Math.round(rect.height),
          };
        }

        // Recurse into children.
        const children = [];
        for (const child of el.children) {
          if (nodeCount >= maxNodes) break;
          const childNode = buildNode(child, depth + 1);
          if (childNode) children.push(childNode);
        }
        if (children.length > 0) {
          node.children = children;
        }

        return node;
      }

      const tree = buildNode(root, 0);

      return {
        tree,
        nodeCount,
        truncated: nodeCount >= maxNodes,
        url: location.href,
        title: document.title,
      };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- upload_image: Simulate file upload ----
  async function handleUploadFile(msg, sendResponse) {
    try {
      const el = resolveElement(msg);
      if (!el) {
        sendResponse({ error: `Element not found: ${msg.selector || msg.somRef}` });
        return;
      }

      if (el.tagName !== 'INPUT' || el.type !== 'file') {
        sendResponse({ error: `Element is not a file input: ${el.tagName} type=${el.type}` });
        return;
      }

      // msg.files: array of { name, type, dataUrl }
      const files = msg.files || [];
      if (files.length === 0) {
        sendResponse({ error: 'No files provided' });
        return;
      }

      const fileObjects = [];
      for (const f of files) {
        // Convert data URL or base64 to File object.
        let blob;
        if (f.dataUrl) {
          const resp = await fetch(f.dataUrl);
          blob = await resp.blob();
        } else if (f.base64) {
          const binary = atob(f.base64);
          const bytes = new Uint8Array(binary.length);
          for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
          blob = new Blob([bytes], { type: f.type || 'application/octet-stream' });
        } else {
          sendResponse({ error: `File "${f.name}" has no dataUrl or base64` });
          return;
        }
        fileObjects.push(new File([blob], f.name || 'file', { type: f.type || blob.type }));
      }

      // Create a DataTransfer to set files on the input.
      const dt = new DataTransfer();
      for (const file of fileObjects) {
        dt.items.add(file);
      }
      el.files = dt.files;

      // Dispatch change and input events.
      el.dispatchEvent(new Event('change', { bubbles: true }));
      el.dispatchEvent(new Event('input', { bubbles: true }));

      sendResponse({
        ok: true,
        count: fileObjects.length,
        names: fileObjects.map((f) => f.name),
      });
    } catch (e) {
      sendResponse({ error: e.message });
    }
  }

  // ---- S2-1: clickElement ----
  function clickElement(msg) {
    try {
      const el = resolveElement(msg);
      if (!el) return { error: `Element not found: ${msg.selector || msg.somRef}` };

      // Scroll into view first.
      el.scrollIntoView({ behavior: 'instant', block: 'center', inline: 'center' });

      // Dispatch full click sequence: mousedown → mouseup → click.
      const rect = el.getBoundingClientRect();
      const cx = rect.x + rect.width / 2;
      const cy = rect.y + rect.height / 2;
      const opts = { bubbles: true, cancelable: true, clientX: cx, clientY: cy };

      el.dispatchEvent(new MouseEvent('mousedown', opts));
      el.dispatchEvent(new MouseEvent('mouseup', opts));
      el.click();

      return { ok: true, tag: el.tagName.toLowerCase(), selector: msg.selector };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S2-2: fillForm ----
  function fillForm(msg) {
    try {
      const el = resolveElement(msg);
      if (!el) return { error: `Element not found: ${msg.selector || msg.somRef}` };

      const value = msg.value !== undefined ? String(msg.value) : '';
      const tag = el.tagName;

      if (tag === 'SELECT') {
        // Find option by value or text.
        let found = false;
        for (const opt of el.options) {
          if (opt.value === value || opt.textContent.trim() === value) {
            opt.selected = true;
            found = true;
            break;
          }
        }
        if (!found) return { error: `Option not found: ${value}` };
        el.dispatchEvent(new Event('change', { bubbles: true }));
        return { ok: true, tag: 'select', value };
      }

      if (tag === 'INPUT' && (el.type === 'checkbox' || el.type === 'radio')) {
        const shouldCheck = value === 'true' || value === '1' || value === 'on';
        if (el.checked !== shouldCheck) {
          el.click();
        }
        return { ok: true, tag: 'input', type: el.type, checked: el.checked };
      }

      // input[text/password/email/...], textarea, contenteditable.
      if (el.isContentEditable) {
        el.focus();
        el.textContent = value;
        el.dispatchEvent(new Event('input', { bubbles: true }));
        return { ok: true, tag: tag.toLowerCase(), contenteditable: true, value };
      }

      // Standard input/textarea — use native setter to bypass React/Vue controlled components.
      const nativeSet = Object.getOwnPropertyDescriptor(
        tag === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype,
        'value'
      )?.set;

      el.focus();
      if (msg.clear !== false) {
        // Clear existing value first.
        if (nativeSet) {
          nativeSet.call(el, '');
        } else {
          el.value = '';
        }
        el.dispatchEvent(new Event('input', { bubbles: true }));
      }

      if (nativeSet) {
        nativeSet.call(el, value);
      } else {
        el.value = value;
      }

      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));

      return { ok: true, tag: tag.toLowerCase(), value };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S2-3: scrollTo ----
  function scrollTo(msg) {
    try {
      if (msg.selector) {
        const el = document.querySelector(msg.selector);
        if (!el) return { error: `Element not found: ${msg.selector}` };
        el.scrollIntoView({
          behavior: msg.smooth ? 'smooth' : 'instant',
          block: msg.block || 'center',
          inline: msg.inline || 'center',
        });
        return { ok: true, selector: msg.selector };
      }

      // Scroll to absolute coordinates.
      const x = msg.x || 0;
      const y = msg.y || 0;
      window.scrollTo({
        left: x,
        top: y,
        behavior: msg.smooth ? 'smooth' : 'instant',
      });
      return { ok: true, x, y };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S2-4: highlightElement ----
  function highlightElement(msg) {
    try {
      const el = resolveElement(msg);
      if (!el) return { error: `Element not found: ${msg.selector || msg.somRef}` };

      const color = msg.color || 'rgba(255, 100, 0, 0.6)';
      const duration = msg.duration || 2000;

      el.scrollIntoView({ behavior: 'instant', block: 'center' });

      const overlay = document.createElement('div');
      overlay.className = '__crabclaw_highlight';
      const rect = el.getBoundingClientRect();
      Object.assign(overlay.style, {
        position: 'fixed',
        left: rect.x + 'px',
        top: rect.y + 'px',
        width: rect.width + 'px',
        height: rect.height + 'px',
        border: `3px solid ${color}`,
        backgroundColor: color.replace(/[\d.]+\)$/, '0.1)'),
        pointerEvents: 'none',
        zIndex: '2147483647',
        borderRadius: '3px',
        transition: 'opacity 0.3s',
      });
      document.body.appendChild(overlay);

      setTimeout(() => {
        overlay.style.opacity = '0';
        setTimeout(() => overlay.remove(), 300);
      }, duration);

      return { ok: true, selector: msg.selector };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S2-5: annotateSOM — Set-of-Mark labeling ----
  let _somRefMap = new Map(); // ref number → element

  function annotateSOM(msg) {
    try {
      // Clear previous annotations first.
      clearAnnotations();
      _somRefMap.clear();

      const root = msg.selector
        ? document.querySelector(msg.selector)
        : document.body;
      if (!root) return { error: 'Root element not found' };

      const maxLabels = msg.maxLabels || 200;
      let refNum = 0;
      const elements = [];

      // Collect all interactive/visible elements.
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT, {
        acceptNode: (node) => {
          if (refNum >= maxLabels) return NodeFilter.FILTER_REJECT;
          if (!isInteractive(node) && !isSemanticBlock(node)) return NodeFilter.FILTER_SKIP;
          const rect = node.getBoundingClientRect();
          if (rect.width === 0 || rect.height === 0) return NodeFilter.FILTER_SKIP;
          // Must be in viewport or close to it.
          if (rect.bottom < -100 || rect.top > window.innerHeight + 100) return NodeFilter.FILTER_SKIP;
          return NodeFilter.FILTER_ACCEPT;
        },
      });

      while (walker.nextNode() && refNum < maxLabels) {
        const node = walker.currentNode;
        refNum++;
        _somRefMap.set(refNum, node);

        const rect = node.getBoundingClientRect();
        const label = document.createElement('div');
        label.className = '__crabclaw_som_label';
        label.textContent = String(refNum);
        Object.assign(label.style, {
          position: 'fixed',
          left: (rect.x - 2) + 'px',
          top: (rect.y - 14) + 'px',
          backgroundColor: '#ff6600',
          color: '#fff',
          fontSize: '10px',
          fontWeight: 'bold',
          padding: '1px 4px',
          borderRadius: '3px',
          zIndex: '2147483647',
          pointerEvents: 'none',
          fontFamily: 'monospace',
          lineHeight: '12px',
          whiteSpace: 'nowrap',
        });
        document.body.appendChild(label);

        // Also add a subtle border to the element.
        const outline = document.createElement('div');
        outline.className = '__crabclaw_som_outline';
        Object.assign(outline.style, {
          position: 'fixed',
          left: rect.x + 'px',
          top: rect.y + 'px',
          width: rect.width + 'px',
          height: rect.height + 'px',
          border: '1px solid rgba(255, 102, 0, 0.5)',
          pointerEvents: 'none',
          zIndex: '2147483646',
        });
        document.body.appendChild(outline);

        elements.push({
          ref: refNum,
          tag: node.tagName.toLowerCase(),
          role: node.getAttribute('role') || inferRole(node) || undefined,
          name: (node.getAttribute('aria-label') || node.textContent || '').trim().slice(0, 80),
          bounds: {
            x: Math.round(rect.x),
            y: Math.round(rect.y),
            w: Math.round(rect.width),
            h: Math.round(rect.height),
          },
        });
      }

      return {
        ok: true,
        count: refNum,
        elements,
      };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S2-6: clearAnnotations ----
  function clearAnnotations() {
    try {
      document.querySelectorAll('.__crabclaw_som_label, .__crabclaw_som_outline, .__crabclaw_highlight')
        .forEach((el) => el.remove());
      return { ok: true };
    } catch (e) {
      return { error: e.message };
    }
  }

  // ---- S2-7: Shadow DOM traversal helper ----
  function querySelectorDeep(selector, root = document) {
    // First try regular querySelector.
    const result = root.querySelector(selector);
    if (result) return result;

    // Traverse shadow roots.
    const allEls = root.querySelectorAll('*');
    for (const el of allEls) {
      if (el.shadowRoot) {
        const found = querySelectorDeep(selector, el.shadowRoot);
        if (found) return found;
      }
    }
    return null;
  }

  // ---- S2-9: Selector cache ----
  const _selectorCache = new Map();
  const CACHE_MAX = 100;
  const CACHE_TTL = 5000; // 5s

  function cachedQuery(selector) {
    const cached = _selectorCache.get(selector);
    if (cached && Date.now() - cached.time < CACHE_TTL && document.contains(cached.el)) {
      return cached.el;
    }
    const el = document.querySelector(selector) || querySelectorDeep(selector);
    if (el) {
      if (_selectorCache.size >= CACHE_MAX) {
        // Evict oldest entry.
        const first = _selectorCache.keys().next().value;
        _selectorCache.delete(first);
      }
      _selectorCache.set(selector, { el, time: Date.now() });
    }
    return el;
  }

  // Invalidate cache on DOM mutations.
  const _cacheObserver = new MutationObserver(() => {
    _selectorCache.clear();
  });
  _cacheObserver.observe(document.documentElement, {
    childList: true,
    subtree: true,
  });

  // ---- Resolve element by selector or SOM ref ----
  function resolveElement(msg) {
    if (msg.somRef && _somRefMap.has(msg.somRef)) {
      return _somRefMap.get(msg.somRef);
    }
    if (msg.selector) {
      return cachedQuery(msg.selector);
    }
    return null;
  }

  function isSemanticBlock(el) {
    return ['H1','H2','H3','H4','H5','H6','P','SECTION','ARTICLE','NAV','MAIN','HEADER','FOOTER'].includes(el.tagName);
  }

  // ---- Helpers ----

  function describeElement(el) {
    const rect = el.getBoundingClientRect();
    const desc = {
      tag: el.tagName.toLowerCase(),
      id: el.id || undefined,
      className: el.className && typeof el.className === 'string'
        ? el.className.slice(0, 200) : undefined,
      text: (el.innerText || el.textContent || '').trim().slice(0, 500),
      href: el.href || undefined,
      src: el.src || undefined,
      value: el.value !== undefined && el.value !== '' ? String(el.value) : undefined,
      type: el.type || undefined,
      role: el.getAttribute('role') || undefined,
      ariaLabel: el.getAttribute('aria-label') || undefined,
      bounds: {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        w: Math.round(rect.width),
        h: Math.round(rect.height),
      },
      visible: rect.width > 0 && rect.height > 0,
    };

    // Strip undefined keys for cleaner output.
    for (const k of Object.keys(desc)) {
      if (desc[k] === undefined) delete desc[k];
    }
    return desc;
  }

  function isInteractive(el) {
    const tag = el.tagName;
    if (['A', 'BUTTON', 'INPUT', 'TEXTAREA', 'SELECT', 'DETAILS', 'SUMMARY'].includes(tag)) {
      return true;
    }
    if (el.getAttribute('role') && ['button', 'link', 'checkbox', 'radio', 'textbox',
        'combobox', 'menuitem', 'tab', 'switch', 'slider'].includes(el.getAttribute('role'))) {
      return true;
    }
    if (el.hasAttribute('tabindex') || el.hasAttribute('onclick') || el.hasAttribute('contenteditable')) {
      return true;
    }
    return false;
  }

  function inferRole(el) {
    const tag = el.tagName;
    const roleMap = {
      A: 'link',
      BUTTON: 'button',
      INPUT: inputRole(el),
      TEXTAREA: 'textbox',
      SELECT: 'combobox',
      IMG: 'img',
      NAV: 'navigation',
      MAIN: 'main',
      HEADER: 'banner',
      FOOTER: 'contentinfo',
      ASIDE: 'complementary',
      FORM: 'form',
      TABLE: 'table',
      UL: 'list',
      OL: 'list',
      LI: 'listitem',
      H1: 'heading',
      H2: 'heading',
      H3: 'heading',
      H4: 'heading',
      H5: 'heading',
      H6: 'heading',
    };
    return roleMap[tag] || '';
  }

  function inputRole(el) {
    switch (el.type) {
      case 'checkbox': return 'checkbox';
      case 'radio': return 'radio';
      case 'range': return 'slider';
      case 'search': return 'searchbox';
      default: return 'textbox';
    }
  }

  console.log('[CrabClaw] Content script loaded:', location.href);
}
