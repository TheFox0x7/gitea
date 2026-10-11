import {initRepoDiffPierreView} from './repo-diff-pierre.ts';

// Spike test: collapsed-context hunk expansion must fetch full file contents
// via loadDiffFiles and reveal the previously hidden lines.

const HEAD = 'h-a1b2c3';
const BASE = 'b-9f8e7d';

const PATCH = `diff --git a/big.txt b/big.txt
index 1111111..2222222 100644
--- a/big.txt
+++ b/big.txt
@@ -1 +1 @@
-old line 1
+new line 1
@@ -100 +100 @@
-old line 100
+new line 100
`;

function makeContent(line1: string, line100: string): string {
  return Array.from({length: 120}, (_, index) => {
    const n = index + 1;
    if (n === 1) return line1;
    if (n === 100) return line100;
    return `line ${n}`;
  }).join('\n');
}

function waitFor(fn: () => boolean, timeout = 20_000): Promise<void> {
  return new Promise((resolve, reject) => {
    const start = performance.now();
    const tick = () => {
      if (fn()) return resolve();
      if (performance.now() - start > timeout) return reject(new Error('timeout'));
      requestAnimationFrame(tick);
    };
    tick();
  });
}

test('spike: hunk expansion fetches full file and reveals collapsed lines', async () => {
  const fetchCalls: string[] = [];
  vi.stubGlobal('fetch', (input: string) => {
    fetchCalls.push(input);
    let body = 'x';
    if (input.includes('/pulls/1.diff')) body = PATCH;
    if (input.includes(`raw/commit/${HEAD}`)) body = makeContent('new line 1', 'new line 100');
    if (input.includes(`raw/commit/${BASE}`)) body = makeContent('old line 1', 'old line 100');
    return Promise.resolve(new Response(body, {status: 200, headers: {'content-type': 'text/plain'}}));
  });

  document.body.innerHTML = '<div id="diff-pierre" class="tw-hidden"></div>';
  const mount = document.querySelector<HTMLElement>('#diff-pierre')!;
  mount.setAttribute('data-global-init', 'initRepoDiffPierreView');
  mount.setAttribute('data-diff-url', '/u/r/pulls/1.diff');
  mount.setAttribute('data-viewed-files-url', '/u/r/pulls/1/viewed-files');
  mount.setAttribute('data-comment-url', '/u/r/pulls/1/files/reviews/comments');
  mount.setAttribute('data-headcommit', HEAD);
  mount.setAttribute('data-before-commit', BASE);
  mount.setAttribute('data-repo-link', '/u/r');
  mount.setAttribute('data-whitespace', 'show-all');
  mount.setAttribute('data-diff-style', 'unified');
  mount.setAttribute('data-style-view-tooltips', JSON.stringify({split: 'S', unified: 'U'}));

  initRepoDiffPierreView(mount);
  await waitFor(() => mount.querySelector('diffs-container') !== null);
  const host = mount.querySelector('diffs-container')!;
  // code lines render asynchronously (shiki-js); wait for real content first
  try {
    await waitFor(() => host.shadowRoot?.textContent?.includes('new line 1') ?? false, 15_000);
  } catch {
    throw new Error(`no content rendered: ${JSON.stringify({light: host.textContent?.slice(0, 300), shadow: host.shadowRoot?.innerHTML.slice(0, 600)})}`);
  }

  // collapsed region between the two hunks must NOT be visible before expanding
  expect(host.shadowRoot?.textContent ?? '').not.contain('line 50');

  // click the separator's "expand both" button with shift -> expand the whole gap
  const expandArea = host.shadowRoot?.querySelector('[data-expand-index] [data-expand-both]') ??
    host.shadowRoot?.querySelector('[data-expand-index] [data-expand-up]') ??
    host.shadowRoot?.querySelector('[data-expand-index]');
  expect(expandArea, `shadow=${JSON.stringify(host.shadowRoot?.innerHTML)}`).toBeTruthy();
  expandArea!.dispatchEvent(new MouseEvent('click', {bubbles: true, composed: true, shiftKey: true}));

  try {
    await waitFor(() => host.shadowRoot?.textContent?.includes('line 50') ?? false, 10_000);
  } catch {
    throw new Error(`expand failed: ${JSON.stringify({fetchCalls, shadowLen: host.shadowRoot?.textContent?.length, shadowHead: host.shadowRoot?.textContent?.slice(0, 300)})}`);
  }
  expect(fetchCalls.some((url) => url.includes('raw/commit/')), JSON.stringify(fetchCalls)).toBe(true);
  expect(host.shadowRoot?.textContent).contain('line 50');
});
