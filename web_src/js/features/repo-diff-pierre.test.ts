import {partitionDiffFiles, type PatchFileLike} from './repo-diff-pierre.ts';

test('partitionDiffFiles', () => {
  const patchFiles: PatchFileLike[] = [
    {name: 'src/main.go', hunks: [{}]}, // modified file
    {name: 'assets/blob.png', hunks: []}, // binary: not renderable
    {name: 'renamed.txt', hunks: []}, // rename-pure: not renderable
    {name: 'src/gone.go', hunks: [{}]}, // deleted file
    {name: 'src/new.go', hunks: [{}]}, // added file
  ];
  const {renderable, skipped} = partitionDiffFiles(patchFiles);
  expect(renderable.map((fileDiff) => fileDiff.name)).toEqual(['src/main.go', 'src/gone.go', 'src/new.go']);
  expect(skipped).toEqual(['assets/blob.png', 'renamed.txt']);
});
