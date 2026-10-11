import type {AnnotationSide, DiffLineAnnotation, FileContents, FileDiff, FileDiffLoadedFiles, FileDiffMetadata, FileDiffOptions} from '@pierre/diffs';
import {applyFiltersToFileBoxes, diffTreeStore, diffTreeStoreSetViewed} from '../modules/diff-file.ts';
import {GET, POST} from '../modules/fetch.ts';
import {initComboMarkdownEditor} from './comp/ComboMarkdownEditor.ts';
import {trString} from '../modules/i18n.ts';
import {createTippy} from '../modules/tippy.ts';
import {pathEscapeSegments} from '../utils/url.ts';
import {svg} from '../svg.ts';
import {showErrorToast} from '../modules/toast.ts';
import {addDelegatedEventListener, createElementFromHTML, hideElem, showElem} from '../utils/dom.ts';
import {isDarkTheme} from '../utils.ts';

// Spike: render the PR "files changed" diff fully client-side with
// @pierre/diffs. The library (and its Shiki dependency) is imported lazily so
// it only loads on diff pages. Binary/image/CSV files are not rendered yet:
// files whose patch has no hunks are skipped and the server-rendered view
// stays as the fallback for anything the client can't handle. The file tree
// keeps working: its filter walks the same containers via diff-file.ts.
//
// Ported server-side behaviors, still in spike quality:
// - per-file collapse, incl. "expand/collapse all files" toolbar buttons
// - mark as viewed (initial state from the server, persisted via POST
//   /viewed-files, folded once viewed, summary + file tree updated)
// - instant unified/split toggle (no page reload)
// - whitespace treatment (server re-runs git diff with the flag, see
//   routers/web/repo/pull.go DownloadPullDiffOrPatch)

const serverDiffSelector = '#diff-file-boxes';
const diffStyleSelector = '.diff-detail-actions a[href^="?style="]';

type DiffStyle = 'unified' | 'split';

// keep the heavy library in a lazy chunk; the module type is inferred from this call
const loadDiffs = () => import('@pierre/diffs');

type CommentMetadata = {
  html: string;
};

type RenderedDiff = {
  name: string;
  container: HTMLElement;
  instance: FileDiff<CommentMetadata>;
};

// the server-rendered diff boxes (kept hidden in the DOM) already contain the
// per-line review comment threads; we scrape their markup and render it into
// the library's annotation rows, one keyed by path -> side:line
const commentsByPath = new Map<string, DiffLineAnnotation<CommentMetadata>[]>();
let commentsScraped = false;

let renderedDiffs: RenderedDiff[] = [];
let clientActive = false;
let currentDiffStyle: DiffStyle = 'unified';
let currentWhitespace = 'show-all';
let styleViewTooltips: Record<DiffStyle, string> = {split: '', unified: ''};

let currentContainer: HTMLElement | undefined;
let currentDiffUrl = '';
let currentViewedStates = new Map<string, boolean>();
let currentViewedFilesUrl = '';
let currentCommentUrl = '';
let currentCommentFormUrl = '';
let currentHeadCommit = '';
let currentBeforeCommit = '';
let currentRepoLink = '';
const fileDiffsByName = new Map<string, FileDiffMetadata>();

export type PatchFileLike = {
  name: string;
  hunks: unknown[];
};

// Files without hunks (binary, rename-only, mode-only, pure moves) can't be
// rendered by a code diff library - skip them like a hybrid view would.
export function partitionDiffFiles<F extends PatchFileLike>(fileDiffs: F[]): {renderable: F[], skipped: string[]} {
  const renderable = fileDiffs.filter((fileDiff) => fileDiff.hunks.length > 0);
  const skipped = fileDiffs.filter((fileDiff) => fileDiff.hunks.length === 0).map((fileDiff) => fileDiff.name);
  return {renderable, skipped};
}

function createSkippedFilesNote(container: HTMLElement, skipped: string[]): void {
  if (skipped.length === 0) return;
  const note = document.createElement('div');
  note.className = 'ui info message';
  note.append(`Spike: ${skipped.length} file(s) have no server-rendered box to display (beyond the initial batch): `, skipped.join(', '));
  container.append(note);
}

function hideServerRenderedDiff() {
  const fileBoxes = document.querySelector(serverDiffSelector);
  if (fileBoxes) hideElem(fileBoxes);
}

function showServerRenderedDiff() {
  const fileBoxes = document.querySelector(serverDiffSelector);
  if (fileBoxes) showElem(fileBoxes);
}

function parseViewedStates(container: HTMLElement): Map<string, boolean> {
  const raw = container.getAttribute('data-viewed-states');
  if (!raw) return new Map();
  try {
    const states = JSON.parse(raw) as Record<string, boolean>;
    return new Map(Object.entries(states).filter(([, viewed]) => viewed));
  } catch {
    return new Map();
  }
}

function refreshViewedFilesSummary(delta: number) {
  const review = window.config.pageData.prReview;
  const progress = document.querySelector<HTMLProgressElement>('#viewed-files-summary');
  if (!review || !progress) return;
  review.numberOfViewedFiles += delta;
  progress.setAttribute('value', String(review.numberOfViewedFiles));
  const summaryLabel = document.querySelector<HTMLElement>('#viewed-files-summary-label');
  if (summaryLabel) {
    const text = summaryLabel.getAttribute('data-text-changed-template')!;
    summaryLabel.textContent = trString(text, review.numberOfViewedFiles, review.numberOfFiles);
  }
}

// the server-rendered diff boxes (hidden but present in the DOM) already carry
// one .conversation-holder per commented line; scrape them so the client renderer
// can place the same markup (reply/resolve/prev-next and their delegated handlers
// keep working) into the library's annotation rows
function scrapeServerComments(): void {
  if (commentsScraped) return;
  commentsScraped = true;
  commentsByPath.clear();
  for (const holder of document.querySelectorAll<HTMLElement>('#diff-file-boxes .conversation-holder')) {
    const path = holder.getAttribute('data-path');
    const line = Number(holder.getAttribute('data-idx'));
    if (!path || !line) continue;
    const side: AnnotationSide = holder.getAttribute('data-side') === 'left' ? 'deletions' : 'additions';
    const annotations = commentsByPath.get(path) ?? [];
    annotations.push({side, lineNumber: line, metadata: {html: holder.outerHTML}});
    commentsByPath.set(path, annotations);
  }
}

function findServerBoxByName(name: string): HTMLElement | undefined {
  return document.querySelector<HTMLElement>(`#diff-file-boxes .diff-file-box[data-new-filename="${CSS.escape(name)}"]`) ?? undefined;
}

function findServerBox(fileDiff: FileDiffMetadata): HTMLElement | undefined {
  return findServerBoxByName(fileDiff.name);
}

function isCsvFileName(name: string): boolean {
  const lower = name.toLowerCase();
  return lower.endsWith('.csv') || lower.endsWith('.tsv'); // mirrors isFileTypeCsv in gitdiff.go
}

// non-text files (images, CSV, submodules, binary/rename/mode-only messages)
// are server-rendered boxes; move them (not clone) into the client list so their
// already-initialized image-diff/tab/view-toggle handlers keep working and no
// duplicate ids appear. The registry survives client re-renders (whitespace etc.).
const movedServerBoxes = new Map<string, HTMLElement>();

function moveServerBoxInto(name: string, container: HTMLElement): boolean {
  let box = movedServerBoxes.get(name);
  if (!box) box = findServerBoxByName(name);
  if (!box) return false;
  box.remove();
  box.classList.add('diff-client-file-box'); // the tree filter drives these too
  container.append(box);
  movedServerBoxes.set(name, box);
  return true;
}

function restoreMovedServerBoxes(): void {
  const fileBoxes = document.querySelector('#diff-file-boxes');
  for (const box of movedServerBoxes.values()) {
    box.remove();
    if (fileBoxes) fileBoxes.append(box);
  }
  movedServerBoxes.clear();
}

// #diff-<hash>/#issuecomment-… anchors and :target must hit the visible client
// containers, not the hidden server copies that carry the same ids; strip those
// ids from whatever stays hidden. Functional ids (loaded via load-more) are kept.
function stripHiddenServerBoxIds(): void {
  const kept = new Set(['diff-incomplete', 'diff-load-progress', 'diff-show-more-files']);
  for (const el of document.querySelectorAll<HTMLElement>('#diff-file-boxes [id]')) {
    const id = el.id;
    // box id plus #diff-source/-rendered-* file bodies start with "diff-"
    if ((id.startsWith('diff-') && !kept.has(id)) || id.startsWith('issuecomment-') || id.startsWith('code-comments-')) {
      el.removeAttribute('id');
    }
  }
}

// file-header extras are scraped from the hidden server-rendered boxes: badges
// (LFS/generated/vendored/entry mode) and the copy-path button go next to the
// filename; protected/changed-since labels and the kebab menu (view/edit/copy
// filename/unescape, with baked URLs) go to the right side. The kebab keeps its
// data-global-init marker, so the global observer wires its tippy on injection.
function scrapeHeaderFilenameSuffix(fileDiff: FileDiffMetadata): HTMLElement | undefined {
  const box = findServerBox(fileDiff);
  if (!box) return undefined;
  const wrapper = document.createElement('div');
  wrapper.className = 'flex-text-block tw-gap-1';
  const copyBtn = box.querySelector<HTMLElement>('.diff-file-name [data-clipboard-text]');
  if (copyBtn) wrapper.append(copyBtn.cloneNode(true));
  for (const badge of box.querySelectorAll<HTMLElement>('.diff-file-name .ui.label')) {
    wrapper.append(badge.cloneNode(true));
  }
  return wrapper.children.length ? wrapper : undefined;
}

function scrapeHeaderMetadataExtras(fileDiff: FileDiffMetadata): HTMLElement | undefined {
  const box = findServerBox(fileDiff);
  if (!box) return undefined;
  const wrapper = document.createElement('div');
  wrapper.className = 'flex-text-block tw-gap-1';
  const protectedLabel = box.querySelector<HTMLElement>('.diff-file-header-actions .ui.basic.label');
  if (protectedLabel) wrapper.append(protectedLabel.cloneNode(true));
  const changedSince = box.querySelector<HTMLElement>('.diff-file-header-actions .changed-since-last-review');
  if (changedSince) wrapper.append(changedSince.cloneNode(true));
  const kebabBtn = box.querySelector<HTMLElement>('.diff-file-header-actions [data-global-init="initDiffHeaderPopupMenu"]');
  if (kebabBtn) {
    // the server tippy (created at page load) already moved the .tippy-target
    // into its popper (appended to body), so the sibling lookup fails; pull the
    // popup content from the live tippy instance and create our own tippy
    const kebabClone = kebabBtn.cloneNode(true) as HTMLElement;
    kebabClone.removeAttribute('data-global-init'); // we create the tippy ourselves below
    wrapper.append(kebabClone);
    const tippy = (kebabBtn as HTMLElement & {_tippy?: {popper: HTMLElement}})._tippy;
    const contentNode = tippy?.popper?.querySelector('.tippy-content') ?? tippy?.popper;
    if (contentNode && contentNode.children.length > 0) {
      const targetClone = document.createElement('div');
      targetClone.className = 'tippy-target';
      for (const child of contentNode.children) targetClone.append(child.cloneNode(true));
      createTippy(kebabClone, {content: targetClone, theme: 'menu', placement: 'bottom-end', trigger: 'click', interactive: true, hideOnClick: true});
    }
  }
  return wrapper.children.length ? wrapper : undefined;
}

// comments are the authoritative threads (scraped + post-submit captures);
// drafts are transient add-comment forms kept separately, so cancelling a draft
// never touches the thread or other runtime state. Formats are server markup.
const draftsByPath = new Map<string, DiffLineAnnotation<CommentMetadata>[]>();

function annotationsFor(fileDiff: FileDiffMetadata): DiffLineAnnotation<CommentMetadata>[] {
  const byKey = new Map<string, DiffLineAnnotation<CommentMetadata>>();
  for (const path of [fileDiff.name, fileDiff.prevName]) {
    if (!path) continue;
    for (const annotation of commentsByPath.get(path) ?? []) {
      byKey.set(`${annotation.side}:${annotation.lineNumber}`, annotation);
    }
  }
  // drafts sit at the line being edited and replace its thread while active
  for (const path of [fileDiff.name, fileDiff.prevName]) {
    if (!path) continue;
    for (const annotation of draftsByPath.get(path) ?? []) {
      byKey.set(`${annotation.side}:${annotation.lineNumber}`, annotation);
    }
  }
  return [...byKey.values()];
}

function upsertComment(path: string, side: AnnotationSide, line: number, html: string): void {
  const annotations = commentsByPath.get(path) ?? [];
  const key = `${side}:${line}`;
  const index = annotations.findIndex((annotation) => `${annotation.side}:${annotation.lineNumber}` === key);
  const annotation: DiffLineAnnotation<CommentMetadata> = {side, lineNumber: line, metadata: {html}};
  if (index === -1) annotations.push(annotation);
  else annotations[index] = annotation;
  commentsByPath.set(path, annotations);
}

function setDraft(path: string, side: AnnotationSide, line: number, html: string): void {
  const drafts = draftsByPath.get(path) ?? [];
  const key = `${side}:${line}`;
  const index = drafts.findIndex((draft) => `${draft.side}:${draft.lineNumber}` === key);
  const draft: DiffLineAnnotation<CommentMetadata> = {side, lineNumber: line, metadata: {html}};
  if (index === -1) drafts.push(draft);
  else drafts[index] = draft;
  draftsByPath.set(path, drafts);
}

// update only the annotation rows of one file; unlike a full rebuild this keeps
// the instance's runtime state (collapse, selection, hunk expansion) intact
function refreshFileAnnotations(path: string): void {
  const fileDiff = fileDiffsByName.get(path);
  const rendered = renderedDiffs.find((entry) => entry.name === path);
  if (!fileDiff || !rendered) return;
  rendered.instance.setLineAnnotations(annotationsFor(fileDiff));
  rendered.instance.rerender();
}

// the add-comment draft uses the same server form (and editor) as the server
// path; the .conversation-holder wrapper is unwrapped so the global
// ".conversation-holder form" submit handler does not double-post - we own the
// submit (submitDraftComment) and keep the thread state authoritative
function unwrapConversationHolder(html: string): string {
  const holder = createElementFromHTML(html);
  const cloud = holder.querySelector<HTMLElement>('.comment-code-cloud');
  const form = (cloud ?? holder).querySelector<HTMLFormElement>('form.comment-form');
  form?.classList.add('diff-draft-form');
  return (cloud ?? holder).outerHTML;
}

async function openCommentForm(path: string, side: AnnotationSide | undefined, line: number): Promise<void> {
  if (!currentContainer || !currentCommentFormUrl || !line) return;
  const sideOrDefault = side ?? 'additions';
  const resp = await GET(currentCommentFormUrl);
  if (!resp.ok) return;
  setDraft(path, sideOrDefault, line, unwrapConversationHolder(await resp.text()));
  refreshFileAnnotations(path);
  const rendered = renderedDiffs.find((entry) => entry.name === path);
  const form = rendered?.container.querySelector<HTMLFormElement>('form.diff-draft-form');
  if (!form) return;
  (form.querySelector<HTMLInputElement>('input[name=line]')!).value = String(line);
  (form.querySelector<HTMLInputElement>('input[name=side]')!).value = sideOrDefault === 'deletions' ? 'previous' : 'proposed';
  (form.querySelector<HTMLInputElement>('input[name=path]')!).value = path;
  const editorContainer = form.querySelector<HTMLElement>('.combo-markdown-editor');
  if (editorContainer) await initComboMarkdownEditor(editorContainer, {});
  form.querySelector('textarea')?.focus();
}

function draftKeyFromForm(form: HTMLFormElement): {path: string, side: AnnotationSide, line: number} {
  return {
    path: form.querySelector<HTMLInputElement>('input[name=path]')!.value,
    side: form.querySelector<HTMLInputElement>('input[name=side]')!.value === 'previous' ? 'deletions' : 'additions',
    line: Number(form.querySelector<HTMLInputElement>('input[name=line]')!.value),
  };
}

function removeDraft(path: string, side: AnnotationSide, line: number): void {
  const drafts = draftsByPath.get(path);
  if (!drafts) return;
  const index = drafts.findIndex((draft) => draft.side === side && draft.lineNumber === line);
  if (index === -1) return;
  drafts.splice(index, 1);
  if (drafts.length === 0) draftsByPath.delete(path);
  refreshFileAnnotations(path); // the backed-up thread was never replaced, so nothing to restore
}

function bumpPendingCounter(): void {
  const counter = document.querySelector('#review-box .review-comments-counter');
  if (!counter) return;
  const num = parseInt(counter.getAttribute('data-pending-comment-number')!) + 1 || 1;
  counter.setAttribute('data-pending-comment-number', String(num));
  counter.textContent = String(num);
}

async function submitDraftComment(form: HTMLFormElement, submitter: HTMLButtonElement | undefined): Promise<void> {
  const textarea = form.querySelector<HTMLTextAreaElement>('textarea[name=content]')!;
  if (!textarea.value.trim()) return;
  const {path, side, line} = draftKeyFromForm(form);
  const submitBtn = submitter ?? form.querySelector<HTMLButtonElement>('button[type=submit]')!;
  submitBtn.disabled = true;
  try {
    // mirror the global handler's payload: the form fields plus the clicked button
    const data = new FormData(form);
    if (submitter?.name) data.append(submitter.name, submitter.value);
    const resp = await POST(form.getAttribute('action')!, {data});
    if (!resp.ok) throw new Error(`comment POST failed: ${resp.status}`);
    upsertComment(path, side, line, await resp.text()); // the new thread fragment
    removeDraft(path, side, line);
    if (submitter?.name === 'pending_review') bumpPendingCounter();
  } catch (error) {
    console.error('Failed to add comment:', error);
    showErrorToast('Failed to add the comment.');
    submitBtn.disabled = false;
  }
}

// Expanding collapsed hunks needs the full file contents on both sides; fetch
// the blobs at the viewed range's commits (the patch itself is partial).
async function loadFilesForExpansion(fileDiff: FileDiffMetadata): Promise<FileDiffLoadedFiles> {
  const fetchFile = async (sha: string, path: string): Promise<FileContents> => {
    const resp = await GET(`${currentRepoLink}/raw/commit/${encodeURIComponent(sha)}/${pathEscapeSegments(path)}`);
    if (!resp.ok) throw new Error(`fetching ${path} failed: ${resp.status}`);
    return {name: path, contents: await resp.text(), cacheKey: `${sha}:${path}`};
  };
  if (fileDiff.type === 'rename-pure') {
    return {oldFile: null, newFile: await fetchFile(currentHeadCommit, fileDiff.name)};
  }
  return {
    oldFile: await fetchFile(currentBeforeCommit, fileDiff.prevName ?? fileDiff.name),
    newFile: await fetchFile(currentHeadCommit, fileDiff.name),
  };
}

function buildFileDiff(fileDiff: FileDiffMetadata, fileContainer: HTMLElement, diffs: Awaited<ReturnType<typeof loadDiffs>>): RenderedDiff {
  const treeStore = window.config.pageData.DiffFileTree ? diffTreeStore() : undefined;
  fileContainer.className = 'diff-client-file-box';
  fileContainer.setAttribute('data-new-filename', fileDiff.name);
  fileContainer.setAttribute('data-old-filename', fileDiff.prevName ?? fileDiff.name);
  const nameHash = treeStore?.fullNameMap[fileDiff.name]?.NameHash;
  if (nameHash) fileContainer.id = `diff-${nameHash}`;

  let instance: FileDiff<CommentMetadata>;
  const options: FileDiffOptions<CommentMetadata, undefined> = {
    theme: diffs.DEFAULT_THEMES,
    themeType: isDarkTheme() ? 'dark' : 'light',
    diffStyle: currentDiffStyle,
    // Gitea's default CSP has no 'wasm-unsafe-eval', so use the JS engine.
    preferredHighlighter: 'shiki-js',
    expansionLineCount: 10,
    enableLineSelection: true,
    stickyHeader: true,
    collapsed: currentViewedStates.get(fileDiff.name) ?? false,
    renderHeaderFilenameSuffix: () => scrapeHeaderFilenameSuffix(fileDiff),
    renderHeaderMetadata: () => {
      const controls = document.createElement('div');
      controls.className = 'flex-text-block tw-gap-1';
      controls.append(createViewedToggle(fileDiff.name, instance), createCollapseToggle(instance));
      const extras = scrapeHeaderMetadataExtras(fileDiff);
      if (extras) controls.append(extras);
      return controls;
    },
    renderAnnotation(annotation) {
      const el = document.createElement('div');
      el.innerHTML = annotation.metadata.html;
      return el;
    },
  };
  if (currentRepoLink && currentHeadCommit && currentBeforeCommit) {
    // hunk expansion re-fetches the full file contents from raw blobs
    options.loadDiffFiles = loadFilesForExpansion;
  }
  if (currentCommentUrl) {
    // the library's built-in gutter "+" (no custom renderGutterUtility, which
    // would both overlap the line number and forbid onGutterUtilityClick)
    options.enableGutterUtility = true;
    options.onGutterUtilityClick = (range) => {
      openCommentForm(fileDiff.name, range.side, range.start);
    };
  }
  instance = new diffs.FileDiff(options);
  instance.render({fileDiff, lineAnnotations: annotationsFor(fileDiff), fileContainer});
  return {name: fileDiff.name, container: fileContainer, instance};
}

function createCollapseToggle(instance: FileDiff<CommentMetadata>): HTMLElement {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'btn interact-fg tw-p-1';
  btn.title = 'Collapse / expand';
  const updateIcon = () => {
    btn.innerHTML = svg(instance.options.collapsed ? 'octicon-chevron-right' : 'octicon-chevron-down', 18);
  };
  updateIcon();
  btn.addEventListener('click', () => {
    instance.setOptions({...instance.options, collapsed: !instance.options.collapsed});
    instance.rerender();
    updateIcon();
  });
  return btn;
}

function createViewedToggle(
  name: string,
  instance: FileDiff<CommentMetadata>,
): HTMLElement {
  // mirror the server's .viewed-file-form checkbox-label styling
  const label = document.createElement('label');
  label.className = 'viewed-file-form';
  const checkbox = document.createElement('input');
  checkbox.type = 'checkbox';
  label.append(checkbox, document.createTextNode(' Viewed'));
  const sync = () => {
    const viewed = currentViewedStates.get(name) ?? false;
    checkbox.checked = viewed;
    label.classList.toggle('viewed-file-checked-form', viewed);
  };
  sync();
  checkbox.addEventListener('change', () => {
    const viewed = checkbox.checked;
    currentViewedStates.set(name, viewed);
    sync();

    const files: Record<string, boolean> = {[name]: viewed};
    POST(currentViewedFilesUrl, {data: {files, headCommitSHA: currentHeadCommit}});

    if (window.config.pageData.DiffFileTree) diffTreeStoreSetViewed(diffTreeStore(), name, viewed);
    refreshViewedFilesSummary(viewed ? 1 : -1);

    // viewed files are folded, like the server-rendered view
    if (viewed && !instance.options.collapsed) {
      instance.setOptions({...instance.options, collapsed: true});
      instance.rerender();
    }
  });
  return label;
}

function clearRendered(container: HTMLElement) {
  for (const rendered of renderedDiffs) {
    rendered.container.remove();
    rendered.instance.cleanUp();
  }
  renderedDiffs = [];
  container.replaceChildren();
}

// "#diff-..."/"#issuecomment-..." permalinks: the client containers/annotations
// render asynchronously, so the existing global hash handler (which runs at page
// init, repo-diff.ts) cannot find them yet - re-run the scroll once rendering
// finishes. Prefer the visible element: the hidden server boxes carry the same ids.
function scrollToPermalinkIfAny(): void {
  const hash = window.location.hash;
  if (!hash.startsWith('#diff-') && !hash.startsWith('#issuecomment-')) return;
  if (document.body.hasAttribute('data-auto-scroll-running')) return;
  const id = hash.slice(1);

  let attempts = 0;
  const tick = () => {
    const el = [...document.querySelectorAll(`#${CSS.escape(id)}`)]
      .find((candidate) => candidate instanceof HTMLElement && candidate.offsetParent !== null);
    if (el instanceof HTMLElement) {
      const cloud = el.closest<HTMLElement>('.comment-code-cloud');
      if (cloud?.classList.contains('tw-hidden')) cloud.classList.remove('tw-hidden'); // reveal resolved threads
      document.body.setAttribute('data-auto-scroll-running', 'true');
      el.scrollIntoView();
      window.location.hash = '';
      window.location.hash = hash; // re-trigger the ":target" highlight (the global handler skips while the guard is set)
      setTimeout(() => document.body.removeAttribute('data-auto-scroll-running'), 0);
      return;
    }
    if (attempts++ < 300) requestAnimationFrame(tick); // budget for the async annotation render
  };
  tick();
}

async function loadAndRender(container: HTMLElement): Promise<void> {
  try {
    const url = currentWhitespace === 'show-all' ? currentDiffUrl : `${currentDiffUrl}?whitespace=${currentWhitespace}`;
    const resp = await GET(url);
    if (!resp.ok) throw new Error(`fetching diff failed: ${resp.status}`);
    const patchText = await resp.text();

    // Keep the heavy library out of the main bundle; vite creates a lazy chunk.
    const diffs = await loadDiffs();
    const {parsePatchFiles} = diffs;
    const parsedPatches = parsePatchFiles(patchText);
    const fileDiffs = parsedPatches.flatMap((patch) => patch.files);

    const {renderable, skipped} = partitionDiffFiles(fileDiffs);
    if (renderable.length === 0) throw new Error('no text diffs to render');

    clearRendered(container);

    scrapeServerComments();
    fileDiffsByName.clear();
    for (const fileDiff of renderable) fileDiffsByName.set(fileDiff.name, fileDiff);

    // CSV files are text (git emits hunks) but get the server's source/rendered
    // view: keep their server box like other non-text files, else render as text
    const csvFiles: FileDiffMetadata[] = [];
    const textFiles: FileDiffMetadata[] = [];
    for (const fileDiff of renderable) {
      (isCsvFileName(fileDiff.name) ? csvFiles : textFiles).push(fileDiff);
    }

    for (const fileDiff of textFiles) {
      const fileContainer = document.createElement(diffs.DIFFS_TAG_NAME);
      container.append(fileContainer);
      renderedDiffs.push(buildFileDiff(fileDiff, fileContainer, diffs));
    }
    for (const fileDiff of csvFiles) {
      if (moveServerBoxInto(fileDiff.name, container)) continue;
      const fileContainer = document.createElement(diffs.DIFFS_TAG_NAME);
      container.append(fileContainer);
      renderedDiffs.push(buildFileDiff(fileDiff, fileContainer, diffs));
    }

    // non-text files keep their server-rendered box (image/csv/submodule/binary
    // messages); only files without a box (e.g. beyond the initial batch) are noted
    const unrendered: string[] = [];
    for (const name of skipped) {
      if (!moveServerBoxInto(name, container)) unrendered.push(name);
    }
    if (unrendered.length > 0) createSkippedFilesNote(container, unrendered);

    // apply any filter restored from the URL (e.g. ?file-filters[]=...)
    if (window.config.pageData.DiffFileTree) applyFiltersToFileBoxes(diffTreeStore());

    hideServerRenderedDiff();
    stripHiddenServerBoxIds();
    clientActive = true;
    syncWhitespaceDropdownUI();
    syncDiffStyleUI();
    showElem(container);
    scrollToPermalinkIfAny();
  } catch (error) {
    console.error('Diff renderer failed, falling back to server-rendered diff:', error);
    showErrorToast('Client-side diff rendering failed, showing the server-rendered diff.');
    clientActive = false;
    clearRendered(container);
    restoreMovedServerBoxes();
    hideElem(container);
    showServerRenderedDiff();
  }
}

function setAllCollapsed(collapsed: boolean) {
  if (!clientActive) return;
  for (const rendered of renderedDiffs) {
    const next = {...rendered.instance.options, collapsed};
    if (rendered.instance.options.collapsed === collapsed) continue;
    rendered.instance.setOptions(next);
    rendered.instance.rerender();
  }
}

function syncWhitespaceDropdownUI() {
  for (const link of document.querySelectorAll<HTMLAnchorElement>(diffStyleSelector)) {
    const radio = link.querySelector<HTMLInputElement>('input[type=radio]');
    if (!radio) continue;
    radio.checked = new URLSearchParams(link.getAttribute('href')!).get('whitespace') === currentWhitespace;
  }
}

// after a client-side style switch the server-baked links/icons would be stale:
// whitespace options keep the current layout, the split/unified toggle (marked
// data-diff-style-toggle) switches to the other one and shows that view's icon +
// tooltip; other style links (e.g. show outdated comments) only get their style
// param refreshed for later full reloads
function syncDiffStyleUI() {
  for (const link of document.querySelectorAll<HTMLAnchorElement>(diffStyleSelector)) {
    const params = new URLSearchParams(link.getAttribute('href')!);
    const isStyleToggle = link.hasAttribute('data-diff-style-toggle');
    const nextStyle: DiffStyle = !isStyleToggle ? currentDiffStyle : currentDiffStyle === 'split' ? 'unified' : 'split';
    params.set('style', nextStyle);
    link.setAttribute('href', `?${params.toString()}`);

    if (!isStyleToggle) continue;
    const otherStyle: DiffStyle = currentDiffStyle === 'split' ? 'unified' : 'split';
    link.innerHTML = svg(otherStyle === 'unified' ? 'gitea-join' : 'gitea-split');
    const tooltip = styleViewTooltips[otherStyle];
    if (!tooltip) continue;
    // initGlobalTooltips has a MutationObserver on data-tooltip-content, so the tippy refreshes
    link.setAttribute('data-tooltip-content', tooltip);
    link.setAttribute('aria-label', tooltip);
  }
}

function initDiffDetailInteractions() {
  // unified/split toggle and the whitespace options are plain links that would
  // reload the page; in the client-rendered view we handle them without a reload
  addDelegatedEventListener(document, 'click', diffStyleSelector, (link, e) => {
    // only the whitespace options and the split/unified toggle are handled in
    // place; other style links (e.g. show outdated comments) need a reload
    const isWhitespaceOption = link.querySelector('input[type=radio]') !== null;
    if (!clientActive || (!isWhitespaceOption && !link.hasAttribute('data-diff-style-toggle'))) return;
    e.preventDefault();
    const params = new URLSearchParams(link.getAttribute('href')!);
    const newStyle = params.get('style');
    const newWhitespace = params.get('whitespace');

    const url = new URL(window.location.href);
    if (newStyle) url.searchParams.set('style', newStyle);
    if (newWhitespace) url.searchParams.set('whitespace', newWhitespace);
    window.history.replaceState(null, '', url);

    if (newWhitespace && newWhitespace !== currentWhitespace) {
      currentWhitespace = newWhitespace;
      loadAndRender(currentContainer!); // re-fetch the patch with the new flag and rebuild
    } else if (newStyle && newStyle !== currentDiffStyle) {
      currentDiffStyle = newStyle === 'split' ? 'split' : 'unified';
      for (const rendered of renderedDiffs) {
        rendered.instance.setOptions({...rendered.instance.options, diffStyle: currentDiffStyle});
        rendered.instance.rerender();
      }
      syncDiffStyleUI();
    }
  });
  document.querySelector('#expand-files-btn')?.addEventListener('click', () => setAllCollapsed(false));
  document.querySelector('#collapse-files-btn')?.addEventListener('click', () => setAllCollapsed(true));

  // permalinks: the global handler scrolls the first "#diff-<hash>" match, which
  // is the hidden server box; re-scroll to the visible client container/comment
  window.addEventListener('hashchange', () => scrollToPermalinkIfAny());

  // the add-comment/reply forms are the real server forms; the draft form is
// unwrapped from .conversation-holder (so the global handler does not double
// post) and submitted here; the reply forms inside threads are handled by the
// global ".conversation-holder form" handler (repo-diff.ts)
  addDelegatedEventListener<HTMLFormElement, SubmitEvent>(document, 'submit', '#diff-pierre form.diff-draft-form', (form, e) => {
    e.preventDefault();
    submitDraftComment(form, e.submitter instanceof HTMLButtonElement ? e.submitter : undefined);
  });
  addDelegatedEventListener<HTMLButtonElement, MouseEvent>(document, 'click', '#diff-pierre form.comment-form button.cancel-code-comment', (btn, e) => {
    e.preventDefault();
    const form = btn.closest<HTMLFormElement>('form.comment-form');
    if (!form) return;
    const {path, side, line} = draftKeyFromForm(form);
    removeDraft(path, side, line);
  });
}

export function initRepoDiffPierreView(container: HTMLElement): void {
  if (container.getAttribute('data-diffs-initialized') === 'true') return;
  container.setAttribute('data-diffs-initialized', 'true');

  commentsScraped = false; // scrape fresh for this page instance (module state is shared across tests)
  currentContainer = container;
  currentDiffUrl = container.getAttribute('data-diff-url') ?? '';
  currentViewedFilesUrl = container.getAttribute('data-viewed-files-url') ?? '';
  currentCommentUrl = container.getAttribute('data-comment-url') ?? '';
  currentCommentFormUrl = container.getAttribute('data-comment-form-url') ?? '';
  currentHeadCommit = container.getAttribute('data-headcommit') ?? '';
  currentBeforeCommit = container.getAttribute('data-before-commit') ?? '';
  currentRepoLink = container.getAttribute('data-repo-link') ?? '';
  currentViewedStates = parseViewedStates(container);
  currentWhitespace = container.getAttribute('data-whitespace') ?? 'show-all';
  currentDiffStyle = container.getAttribute('data-diff-style') === 'split' ? 'split' : 'unified';
  const tooltipsRaw = container.getAttribute('data-style-view-tooltips');
  if (tooltipsRaw) {
    try {
      styleViewTooltips = {...styleViewTooltips, ...JSON.parse(tooltipsRaw)};
    } catch {
      // keep empty tooltips, the toggle icon alone still works
    }
  }
  if (!currentDiffUrl) return; // no data source, keep the server-rendered diff

  initDiffDetailInteractions();
  loadAndRender(container);
}
