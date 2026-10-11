import {initRepoDiffPierreView} from './repo-diff-pierre.ts';

// Spike test: "#issuecomment-..." permalinks scroll to the VISIBLE comment in
// the client-rendered annotation, not the hidden server-rendered copy.

const PATCH = `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
`;

function waitFor(fn: () => boolean, timeout = 15_000): Promise<void> {
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

afterEach(() => {
  vi.restoreAllMocks();
  window.location.hash = '';
});

test('spike: hidden server box ids stripped so #diff- anchors hit the visible container', async () => {
  const scrolled: Element[] = [];
  vi.spyOn(Element.prototype, 'scrollIntoView').mockImplementation(function(this: Element) {
    scrolled.push(this);
  });
  vi.stubGlobal('fetch', () => Promise.resolve(new Response(PATCH, {status: 200, headers: {'content-type': 'text/plain'}})));

  document.body.innerHTML = `
    <div id="diff-file-boxes" hidden class="tw-hidden">
      <div class="diff-file-box" id="diff-abc" data-new-filename="a.txt">
        <div class="diff-file-header"><div class="diff-file-name">a.txt</div></div>
        <div class="diff-file-body">
          <div class="file-body" id="diff-source-abc">src</div>
          <div class="file-body" id="diff-rendered-abc">rendered</div>
        </div>
      </div>
    </div>
    <div id="diff-pierre" class="tw-hidden"></div>
  `;
  const mount = document.querySelector<HTMLElement>('#diff-pierre')!;
  mount.setAttribute('data-global-init', 'initRepoDiffPierreView');
  mount.setAttribute('data-diff-url', '/u/r/pulls/1.diff');
  mount.setAttribute('data-viewed-files-url', '/u/r/pulls/1/viewed-files');
  mount.setAttribute('data-headcommit', 'abc');
  mount.setAttribute('data-whitespace', 'show-all');
  mount.setAttribute('data-diff-style', 'unified');
  mount.setAttribute('data-style-view-tooltips', JSON.stringify({split: 'S', unified: 'U'}));

  initRepoDiffPierreView(mount);
  await waitFor(() => mount.querySelector('diffs-container') !== null);

  // the hidden server box and its file bodies no longer claim the hash targets
  expect(document.querySelector('#diff-file-boxes [id="diff-abc"]')).toBeNull();
  expect(document.querySelector('#diff-file-boxes [id="diff-source-abc"]')).toBeNull();

  // give the (visible) client container the id the tree would assign, then navigate
  (mount.querySelector('diffs-container') as HTMLElement).id = 'diff-abc';
  window.location.hash = '#diff-abc';
  await waitFor(() => scrolled.length > 0);
  expect(scrolled[0]).toBe(mount.querySelector('diffs-container'));
});

test('spike: #issuecomment permalink scrolls the visible annotation copy', async () => {
  const scrolled: Element[] = [];
  vi.spyOn(Element.prototype, 'scrollIntoView').mockImplementation(function(this: Element) {
    scrolled.push(this);
  });
  vi.stubGlobal('fetch', () => Promise.resolve(new Response(PATCH, {status: 200, headers: {'content-type': 'text/plain'}})));

  document.body.innerHTML = `
    <div id="diff-file-boxes" hidden class="tw-hidden">
      <div class="diff-file-box" data-new-filename="a.txt">
        <table class="chroma">
          <tr class="add-comment">
            <td>
              <div class="conversation-holder" data-path="a.txt" data-side="right" data-idx="1">
                <div class="comment-code-cloud"><div class="comment" id="issuecomment-7">COMMENT-7</div></div>
              </div>
            </td>
          </tr>
        </table>
      </div>
    </div>
    <div id="diff-pierre" class="tw-hidden"></div>
  `;
  const mount = document.querySelector<HTMLElement>('#diff-pierre')!;
  mount.setAttribute('data-global-init', 'initRepoDiffPierreView');
  mount.setAttribute('data-diff-url', '/u/r/pulls/1.diff');
  mount.setAttribute('data-viewed-files-url', '/u/r/pulls/1/viewed-files');
  mount.setAttribute('data-headcommit', 'abc');
  mount.setAttribute('data-whitespace', 'show-all');
  mount.setAttribute('data-diff-style', 'unified');
  mount.setAttribute('data-style-view-tooltips', JSON.stringify({split: 'S', unified: 'U'}));

  window.location.hash = '#issuecomment-7'; // initial permalink load
  initRepoDiffPierreView(mount);
  await waitFor(() => mount.querySelector('diffs-container') !== null);

  try {
    await waitFor(() => scrolled.length > 0, 10_000);
  } catch {
    const copies = [...document.querySelectorAll<HTMLElement>('[id="issuecomment-7"]')].map((el) => ({inMount: mount.contains(el), hidden: el.offsetParent === null, parent: el.parentElement?.className ?? ''}));
    throw new Error(`no scroll: copies=${JSON.stringify(copies)} text=${document.body.textContent?.slice(0, 300)} boxes=${document.querySelector('#diff-file-boxes')?.innerHTML ?? ''}`);
  }
  const target = scrolled[0];
  expect(target?.id).eq('issuecomment-7');
  // the server-rendered copy is hidden; the scrolled one must live in the client render
  expect(mount.contains(target), `scrolled id=${target?.id} parent=${target?.parentElement?.className ?? 'none'}`).toBe(true);
  expect(document.querySelector('#diff-file-boxes')!.contains(target)).toBe(false);
});
