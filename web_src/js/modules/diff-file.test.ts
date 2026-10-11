import {applyFiltersToFileBoxes, reactiveDiffTreeStore, type DiffFileTreeData} from './diff-file.ts';

function makeTree(): DiffFileTreeData {
  return {
    TreeRoot: {
      FullName: '', OldFullName: '', DisplayName: '', NameHash: '', DiffStatus: '', EntryMode: '', FileIcon: '', IsViewed: false, Children: [
        {FullName: 'src/a.go', OldFullName: '', DisplayName: 'a.go', NameHash: 'H1', DiffStatus: 'modified', EntryMode: 'blob', FileIcon: '', IsViewed: false, Children: null},
        {FullName: 'src/b.txt', OldFullName: '', DisplayName: 'b.txt', NameHash: 'H2', DiffStatus: 'added', EntryMode: 'blob', FileIcon: '', IsViewed: false, Children: null},
      ],
    },
  };
}

function renderClientContainers() {
  document.body.innerHTML = `
    <div id="diff-no-matches" class="tw-hidden"></div>
    <div id="diff-pierre">
      <diffs-container id="diff-H1" class="diff-client-file-box" data-new-filename="src/a.go" data-old-filename="src/a.go"></diffs-container>
      <diffs-container id="diff-H2" class="diff-client-file-box" data-new-filename="src/b.txt" data-old-filename="src/b.txt"></diffs-container>
    </div>
  `;
  return Array.from(document.querySelectorAll<HTMLElement>('#diff-pierre .diff-client-file-box'));
}

test('applyFiltersToFileBoxes drives client-rendered containers', () => {
  const containers = renderClientContainers();
  const store = reactiveDiffTreeStore(makeTree(), '', '');

  applyFiltersToFileBoxes(store); // no filter
  for (const container of containers) expect(container.classList.contains('tw-hidden')).toBe(false);

  store.filenameFilterQuery = 'a.go';
  applyFiltersToFileBoxes(store);
  expect(containers[0].classList.contains('tw-hidden')).toBe(false);
  expect(containers[1].classList.contains('tw-hidden')).toBe(true);

  store.filenameFilterQuery = 'zzz'; // matches nothing
  applyFiltersToFileBoxes(store);
  for (const container of containers) expect(container.classList.contains('tw-hidden')).toBe(true);
  expect(document.querySelector('#diff-no-matches')!.classList.contains('tw-hidden')).toBe(false);
});

test('applyFiltersToFileBoxes without containers keeps server-path behavior', () => {
  document.body.innerHTML = '<div id="diff-no-matches" class="tw-hidden"></div>';
  const store = reactiveDiffTreeStore(makeTree(), '', '');

  store.filenameFilterQuery = 'zzz';
  applyFiltersToFileBoxes(store); // no containers, matchingCount is 0 -> show "no matches"
  expect(document.querySelector('#diff-no-matches')!.classList.contains('tw-hidden')).toBe(false);

  store.filenameFilterQuery = 'a.go';
  applyFiltersToFileBoxes(store); // tree matches but nothing rendered -> stay hidden (client renderer is not active)
  expect(document.querySelector('#diff-no-matches')!.classList.contains('tw-hidden')).toBe(true);
});
