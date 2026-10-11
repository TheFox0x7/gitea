import {initRepoDiffPierreView} from './repo-diff-pierre.ts';

// Spike test: non-text files (image, binary) keep their server-rendered boxes:
// they are moved (not cloned) into the client list so their already-initialized
// image-diff handlers keep working and no duplicate ids appear.

const PATCH = `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
diff --git a/data.csv b/data.csv
index 1111111..2222222 100644
--- a/data.csv
+++ b/data.csv
@@ -1 +1 @@
-a,b
+a,c
diff --git a/img.png b/img.png
index 1111111..2222222 100644
Binary files a/img.png and b/img.png differ
diff --git a/bin.dat b/bin.dat
index 1111111..2222222 100644
Binary files a/bin.dat and b/bin.dat differ
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

test('spike: non-text files keep their server-rendered boxes', async () => {
  vi.stubGlobal('fetch', () => Promise.resolve(new Response(PATCH, {status: 200, headers: {'content-type': 'text/plain'}})));

  document.body.innerHTML = `
    <div id="diff-file-boxes" hidden class="tw-hidden">
      <div class="diff-file-box" data-new-filename="a.txt">
        <div class="diff-file-header"><div class="diff-file-name">a.txt</div></div>
        <div class="diff-file-body"><table class="chroma"><tr><td>text rows</td></tr></table></div>
      </div>
      <div class="diff-file-box" data-new-filename="data.csv">
        <div class="diff-file-header"><div class="diff-file-name">data.csv</div>
          <div class="ui compact icon buttons" data-global-init="initDiffFileViewToggle">
            <button type="button" class="file-view-toggle" data-toggle-selector="#diff-source-csv1">src</button>
            <button type="button" class="file-view-toggle active" data-toggle-selector="#diff-rendered-csv1">rend</button>
          </div>
        </div>
        <div class="diff-file-body">
          <div id="diff-source-csv1" class="file-body">source table</div>
          <div id="diff-rendered-csv1" class="file-body"><table class="data-table"><tr><td>CSV-TABLE</td></tr></table></div>
        </div>
      </div>
      <div class="diff-file-box" data-new-filename="img.png">
        <div class="diff-file-header"><div class="diff-file-name">img.png</div></div>
        <div class="diff-file-body">
          <div class="image-diff" data-global-init="initDiffFileImageDiff" data-path-after="/u/r/raw/commit/HEAD/img.png">
            <span class="before-container"><img class="image-before"></span>
            <span class="after-container"><img class="image-after"></span>
          </div>
        </div>
      </div>
      <div class="diff-file-box" data-new-filename="bin.dat">
        <div class="diff-file-header"><div class="diff-file-name">bin.dat</div></div>
        <div class="diff-file-body"><div class="tw-p-3">BIN-NOT-SHOWN</div></div>
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

  // text file is a library container
  expect(mount.querySelector('.diff-file-box[data-new-filename="a.txt"]')).toBeNull();
  expect(mount.querySelector('diffs-container')).toBeTruthy();

  // image + binary + csv moved into the client list, intact, tagged for the tree filter
  const imgBox = mount.querySelector<HTMLElement>('.diff-file-box[data-new-filename="img.png"]');
  expect(imgBox, `mount=${mount.innerHTML.slice(0, 400)}`).toBeTruthy();
  expect(imgBox?.classList.contains('diff-client-file-box')).toBe(true);
  expect(imgBox?.querySelector('.image-diff[data-global-init="initDiffFileImageDiff"]')).toBeTruthy();
  expect(imgBox?.querySelector('.image-diff')?.getAttribute('data-path-after')).eq('/u/r/raw/commit/HEAD/img.png');
  expect(mount.querySelector('.diff-file-box[data-new-filename="bin.dat"]')?.textContent).contain('BIN-NOT-SHOWN');

  // csv has hunks (a text file) but keeps its server box WITH the source/rendered toggle
  const csvBox = mount.querySelector<HTMLElement>('.diff-file-box[data-new-filename="data.csv"]');
  expect(csvBox, 'csv box should be moved, not rendered as a library container').toBeTruthy();
  expect(csvBox?.querySelector('[data-global-init="initDiffFileViewToggle"]')).toBeTruthy();
  expect(csvBox?.querySelector('.data-table')?.textContent, `csvBox=${JSON.stringify(csvBox?.outerHTML)}`).contain('CSV-TABLE');
  // no library container for the csv file
  expect([...mount.querySelectorAll('diffs-container')].every((el) => !el.getAttribute('data-new-filename')?.includes('data.csv'))).toBe(true);

  // originals no longer in the hidden list
  expect(document.querySelector('#diff-file-boxes .diff-file-box[data-new-filename="img.png"]')).toBeNull();
  expect(document.querySelector('#diff-file-boxes .diff-file-box[data-new-filename="bin.dat"]')).toBeNull();
  expect(document.querySelector('#diff-file-boxes .diff-file-box[data-new-filename="data.csv"]')).toBeNull();
});
