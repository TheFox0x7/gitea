import {initRepoDiffView} from './repo-diff.ts';
import {initRepoDiffPierreView} from './repo-diff-pierre.ts';

// Spike tests: existing review comments render as annotations, and the comment
// form (the real server form fragment) submits through the global
// ".conversation-holder form" handler without navigating away.

const PATCH = `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
`;

const NEW_THREAD_HTML = '<div class="conversation-holder" data-path="a.txt" data-side="right" data-idx="1">NEW-THREAD</div>';

const FORM_HTML = `<div class="conversation-holder">
  <div class="field comment-code-cloud">
    <form class="ui form comment-form" action="/u/r/pulls/1/files/reviews/comments" method="post">
      <input type="hidden" name="origin" value="diff">
      <input type="hidden" name="side">
      <input type="hidden" name="line">
      <input type="hidden" name="path">
      <textarea name="content">my comment</textarea>
      <button name="single_review" value="true" type="submit">Comment</button>
    </form>
  </div>
</div>`;

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

function setUpMount(): HTMLElement {
  document.body.innerHTML = `
    <div id="diff-file-boxes">
      <div class="diff-file-box" data-new-filename="a.txt">
        <table class="chroma">
          <tr class="add-comment">
            <td><div class="conversation-holder" data-path="a.txt" data-side="right" data-idx="1">COMMENT-TEXT-HERE</div></td>
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
  mount.setAttribute('data-comment-url', '/u/r/pulls/1/files/reviews/comments');
  mount.setAttribute('data-comment-form-url', '/u/r/pulls/1/files/reviews/new_comment');
  mount.setAttribute('data-headcommit', 'abc');
  mount.setAttribute('data-whitespace', 'show-all');
  mount.setAttribute('data-diff-style', 'unified');
  mount.setAttribute('data-style-view-tooltips', JSON.stringify({split: 'S', unified: 'U'}));
  return mount;
}

test('spike: existing comments render as annotations', async () => {
  vi.stubGlobal('fetch', () => Promise.resolve(new Response(PATCH, {status: 200, headers: {'content-type': 'text/plain'}})));

  const mount = setUpMount();
  initRepoDiffPierreView(mount);
  await waitFor(() => mount.querySelector('diffs-container') !== null);

  const host = mount.querySelector('diffs-container')!;
  // the comment thread must be slotted into THIS container (not the original server box)
  await waitFor(() => host.textContent?.includes('COMMENT-TEXT') ?? false);
  expect(host.textContent, `host=${JSON.stringify(host.outerHTML)}`).contain('COMMENT-TEXT');
});

test('spike: comment form submits through the global handler without navigation', async () => {
  // registers the document-delegated ".conversation-holder form" submit handler
  initRepoDiffView();

  const fetchCalls: string[] = [];
  vi.stubGlobal('fetch', (input: string) => {
    fetchCalls.push(input);
    let body = 'x';
    if (input.includes('/pulls/1.diff')) body = PATCH;
    if (input.includes('/files/reviews/new_comment')) body = FORM_HTML;
    if (input.includes('/files/reviews/comments')) body = NEW_THREAD_HTML; // the POST response
    return Promise.resolve(new Response(body, {status: 200, headers: {'content-type': 'text/plain'}}));
  });

  const mount = setUpMount();
  initRepoDiffPierreView(mount);
  await waitFor(() => mount.querySelector('diffs-container') !== null);

  // the draft form is what openCommentForm would inject (conversation-holder + form)
  const holder = document.createElement('div');
  holder.innerHTML = FORM_HTML;
  holder.querySelector<HTMLInputElement>('input[name=side]')!.value = 'proposed';
  holder.querySelector<HTMLInputElement>('input[name=line]')!.value = '1';
  holder.querySelector<HTMLInputElement>('input[name=path]')!.value = 'a.txt';
  mount.append(holder);

  const form = holder.querySelector<HTMLFormElement>('form')!;
  const errors: string[] = [];
  const origError = console.error;
  console.error = (...args) => { errors.push(args.map(String).join(' ')); origError(...args) };
  const notNavigated = form.dispatchEvent(new SubmitEvent('submit', {bubbles: true, cancelable: true,
    submitter: form.querySelector('button') as HTMLButtonElement}));
  expect(notNavigated).toBe(false); // the global handler intercepted - no native navigation

  try {
    await waitFor(() => [...mount.querySelectorAll('.conversation-holder')].some((h) => h.textContent?.includes('NEW-THREAD') ?? false), 10_000);
  } catch {
    throw new Error(`submit failed: ${JSON.stringify({errors, fetchCalls, holderHTML: holder.innerHTML.slice(0, 300)})}`);
  }
  expect(fetchCalls.some((url) => url.includes('/files/reviews/comments')), JSON.stringify(fetchCalls)).toBe(true);
  expect([...mount.querySelectorAll('.conversation-holder')].map((h) => h.textContent).join(' ')).contain('NEW-THREAD');
});
