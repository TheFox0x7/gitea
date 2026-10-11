import {initRepoDiffPierreView} from './repo-diff-pierre.ts';
import {createTippy} from '../modules/tippy.ts';

// Spike test: file-header extras (badges, copy-path, kebab menu, labels) are
// scraped from the hidden server boxes and slotted into the library header.

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

test('spike: file header keeps badges, copy-path and the kebab menu', async () => {
  vi.stubGlobal('fetch', () => Promise.resolve(new Response(PATCH, {status: 200, headers: {'content-type': 'text/plain'}})));

  document.body.innerHTML = `
    <div id="diff-file-boxes">
      <div class="diff-file-box" data-new-filename="a.txt">
        <div class="diff-file-header sticky-2nd-row ui top attached header">
          <div class="diff-file-name tw-flex tw-flex-1 tw-items-center tw-gap-1 tw-flex-wrap">
            <div class="flex-text-block">
              <button type="button" class="fold-file btn interact-bg">
                <svg class="svg octicon-chevron-down"></svg>
              </button>
              <a class="muted file-link tw-font-mono" href="#diff-abc">a.txt</a>
            </div>
            <button type="button" class="btn interact-fg tw-p-2 tw-shrink-0" data-clipboard-text="a.txt" data-tooltip-content="Copy path"><svg class="svg octicon-copy"></svg></button>
            <span class="ui label">LFS</span>
            <span class="ui label">generated</span>
          </div>
          <div class="diff-file-header-actions">
            <span class="ui basic label">protected</span>
            <span class="changed-since-last-review unselectable not-mobile">changed</span>
            <button type="button" class="btn tw-p-1" data-global-init="initDiffHeaderPopupMenu"><svg class="svg octicon-kebab-horizontal"></svg></button>
            <div class="tippy-target">
              <button type="button" class="unescape-button item">unescape</button>
              <button type="button" class="item" data-clipboard-text="a.txt">Copy filename</button>
              <a class="item" rel="nofollow" href="/u/r/src/branch/main/a.txt">View file</a>
            </div>
          </div>
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

  // simulate the server page-init: the kebab tippy is created and moves its
  // .tippy-target into the popper (as Gitea's initDiffHeaderPopupMenu does)
  const fixtureKebab = document.querySelector<HTMLElement>('.diff-file-box[data-new-filename="a.txt"] [data-global-init="initDiffHeaderPopupMenu"]')!;
  const fixtureTarget = fixtureKebab.nextElementSibling as HTMLElement;
  createTippy(fixtureKebab, {content: fixtureTarget, theme: 'menu', placement: 'bottom-end', trigger: 'click', interactive: true, hideOnClick: true});

  initRepoDiffPierreView(mount);
  await waitFor(() => mount.querySelector('diffs-container') !== null);
  const host = mount.querySelector('diffs-container')!;
  await waitFor(() => (host.textContent ?? '').includes('LFS'));

  // badges + copy-path next to the filename
  const suffixSlot = host.querySelector('[slot="header-filename-suffix"]');
  expect(suffixSlot?.textContent, `suffix=${JSON.stringify(suffixSlot?.textContent)}`).contain('LFS');
  expect(suffixSlot?.textContent).contain('generated');
  expect(suffixSlot?.querySelector('[data-clipboard-text="a.txt"]')).toBeTruthy();

  // protected + changed-since on the right
  const metadataSlot = host.querySelector('[slot="header-metadata"]');
  expect(metadataSlot?.textContent, `meta=${JSON.stringify(metadataSlot?.textContent)}`).contain('protected');
  expect(metadataSlot?.textContent).contain('changed');

  // the kebab clone: no data-global-init (we create its tippy ourselves) and the
  // popup content comes from the live tippy of the original button
  expect(metadataSlot?.querySelector('[data-global-init]')).toBeNull();
  const kebabBtn = [...(metadataSlot?.querySelectorAll<HTMLElement>('.btn') ?? [])].find((btn) => '_tippy' in btn);
  expect(kebabBtn).toBeTruthy();
  const tippyContent = (kebabBtn as HTMLElement & {_tippy?: {popper: HTMLElement}})._tippy;
  expect(tippyContent?.popper.textContent).contain('View file');
  expect(tippyContent?.popper.textContent).contain('Copy filename');
});
