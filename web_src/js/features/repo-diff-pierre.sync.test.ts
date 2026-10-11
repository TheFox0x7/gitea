import {initRepoDiffPierreView} from './repo-diff-pierre.ts';

// Spike test: style/whitespace dropdown links must stay in sync after
// client-side switches (no page reload).

const PATCH = `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
`;

function waitFor(fn: () => boolean, timeout = 30_000): Promise<void> {
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

test('spike: unified/split toggle and whitespace options stay in sync', async () => {
  const fetchCalls: string[] = [];
  vi.stubGlobal('fetch', (input: string) => {
    fetchCalls.push(input);
    return Promise.resolve(new Response(PATCH, {status: 200, headers: {'content-type': 'text/plain'}}));
  });

  document.body.innerHTML = `
    <div class="diff-detail-actions">
      <a class="item" href="?style=unified&whitespace=show-all&show-outdated=false"><label><input type="radio">show all</label></a>
      <a id="ws-ignore" class="item" href="?style=unified&whitespace=ignore-all&show-outdated=false"><label><input type="radio">ignore all</label></a>
      <a id="toggle" class="ui tiny basic button" href="?style=split&whitespace=show-all&show-outdated=false" data-diff-style-toggle data-tooltip-content="Show split view">icon</a>
      <a id="outdated" class="item" href="?style=unified&whitespace=show-all&show-outdated=true"><label>show outdated</label></a>
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
  mount.setAttribute('data-style-view-tooltips', JSON.stringify({split: 'Show split view', unified: 'Show unified view'}));

  initRepoDiffPierreView(mount);
  await waitFor(() => mount.querySelector('diffs-container') !== null);
  expect(mount.classList.contains('tw-hidden')).toBe(false);

  const toggle = document.querySelector<HTMLAnchorElement>('#toggle')!;
  const wsIgnore = document.querySelector<HTMLAnchorElement>('#ws-ignore')!;

  // initial state: unified active, toggle advertises switching to split
  expect(toggle.getAttribute('href')).contain('style=split');
  expect(toggle.innerHTML).contain('gitea-split');
  expect(toggle.getAttribute('data-tooltip-content')).eq('Show split view');
  expect(wsIgnore.getAttribute('href')).contain('style=unified');

  // click toggle -> split, no reload, icon/tooltip flip to "show unified"
  toggle.click();
  expect(toggle.getAttribute('href')).contain('style=unified');
  expect(toggle.innerHTML).contain('gitea-join');
  expect(toggle.getAttribute('data-tooltip-content')).eq('Show unified view');
  expect(wsIgnore.getAttribute('href')).contain('style=split');
  expect(new URL(window.location.href).searchParams.get('style')).eq('split');

  // whitespace option refetches with the flag and keeps the split layout
  const fetchCount = fetchCalls.length;
  wsIgnore.click();
  await waitFor(() => fetchCalls.length > fetchCount);
  expect(fetchCalls.at(-1)).contain('whitespace=ignore-all');
  expect(toggle.getAttribute('href')).contain('style=unified');
  expect(new URL(window.location.href).searchParams.get('whitespace')).eq('ignore-all');

  // regression: the options-menu "show outdated comments" link (radio-less, no
  // style toggle marker) must keep its label and only get its style refreshed,
  // never be turned into a duplicate split/unified button
  const outdated = document.querySelector<HTMLAnchorElement>('#outdated')!;
  expect(outdated.innerHTML).contain('show outdated');
  expect(outdated.querySelector('svg')).toBeNull();
  expect(outdated.getAttribute('href')).contain('style=split'); // refreshed to the current style
});
