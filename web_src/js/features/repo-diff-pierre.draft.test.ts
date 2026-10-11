import {initRepoDiffPierreView} from './repo-diff-pierre.ts';

// Spike tests for the draft-comment rework:
// - library: swapping annotations via setLineAnnotations + rerender preserves
//   per-instance state (collapsed), no full rebuild
// - module: the unwrapped draft form is submitted by our own handler (no
//   navigation, form-urlencoded payload with the correct fields)

const PATCH = `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
`;

type CommentMetadata = {html: string};

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

test('lib: setLineAnnotations + rerender swaps annotations and keeps collapsed', async () => {
  const {FileDiff, DIFFS_TAG_NAME, DEFAULT_THEMES, parsePatchFiles} = await import('@pierre/diffs');
  const fileDiff = parsePatchFiles(PATCH).flatMap((patch) => patch.files)[0];
  const root = document.createElement('div');
  document.body.append(root);
  const fileContainer = document.createElement(DIFFS_TAG_NAME);
  root.append(fileContainer);

  const instance = new FileDiff<CommentMetadata>({
    theme: DEFAULT_THEMES,
    themeType: 'light',
    diffStyle: 'unified',
    preferredHighlighter: 'shiki-js',
    renderAnnotation(annotation) {
      const el = document.createElement('div');
      el.innerHTML = annotation.metadata.html;
      return el;
    },
  }, undefined);
  instance.render({fileDiff, lineAnnotations: [{side: 'additions', lineNumber: 1, metadata: {html: 'ANN-A'}}], fileContainer});
  await waitFor(() => fileContainer.textContent?.includes('ANN-A') ?? false);

  // swap the annotation in place; the per-instance state (expanded) must survive
  instance.setLineAnnotations([{side: 'additions', lineNumber: 1, metadata: {html: 'ANN-B'}}]);
  instance.rerender();
  expect(instance.options.collapsed !== true).toBe(true); // still expanded after the swap
  await waitFor(() => fileContainer.textContent?.includes('ANN-B') ?? false);
  expect(fileContainer.textContent).contain('ANN-B');
  expect(fileContainer.textContent).not.contain('ANN-A');

  // collapsing the file must survive a later annotation swap too
  instance.setOptions({...instance.options, collapsed: true});
  instance.rerender();
  instance.setLineAnnotations([{side: 'additions', lineNumber: 1, metadata: {html: 'ANN-C'}}]);
  instance.rerender();
  expect(instance.options.collapsed).toBe(true);
});

test('spike: draft form submit is intercepted and posts the right payload', async () => {
  const fetchCalls: Array<{url: string, body?: FormData}> = [];
  vi.stubGlobal('fetch', (input: string, init?: RequestInit) => {
    fetchCalls.push({url: input, body: init?.body instanceof FormData ? init.body : undefined});
    let body = 'x';
    if (input.includes('/pulls/1.diff')) body = PATCH;
    if (input.includes('/files/reviews/comments')) body = '<div class="conversation-holder">NEW-THREAD</div>';
    return Promise.resolve(new Response(body, {status: 200, headers: {'content-type': 'text/plain'}}));
  });

  document.body.innerHTML = '<div id="diff-pierre" class="tw-hidden"></div>';
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

  // an unwrapped draft form, as openCommentForm injects it (no .conversation-holder)
  const form = document.createElement('form');
  form.className = 'ui form comment-form diff-draft-form';
  form.setAttribute('action', '/u/r/pulls/1/files/reviews/comments');
  form.innerHTML = `
    <input type="hidden" name="origin" value="diff">
    <input type="hidden" name="side" value="proposed">
    <input type="hidden" name="line" value="1">
    <input type="hidden" name="path" value="a.txt">
    <textarea name="content">my comment</textarea>
    <button name="single_review" value="true" type="submit">Comment</button>
  `;
  // the spike reloads annotations from its own state, so seed the draft there via
  // openCommentForm's wiring is not reachable here; the form element must exist in
  // the mount for the delegated listener (initRepoDiffPierreView wires it)
  document.querySelector('#diff-pierre')!.append(form);

  const notNavigated = form.dispatchEvent(new SubmitEvent('submit', {bubbles: true, cancelable: true,
    submitter: form.querySelector('button') as HTMLButtonElement}));
  expect(notNavigated).toBe(false); // our handler intercepted - no native navigation

  await waitFor(() => fetchCalls.some((call) => call.url.includes('/files/reviews/comments')));
  const post = fetchCalls.find((call) => call.url.includes('/files/reviews/comments'))!;
  expect(post.body?.get('content')).eq('my comment');
  expect(post.body?.get('single_review')).eq('true');
  expect(post.body?.get('side')).eq('proposed');
  expect(post.body?.get('line')).eq('1');
});
