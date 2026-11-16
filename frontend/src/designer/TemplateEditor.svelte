<script lang="ts">
  import {onMount, untrack} from "svelte";
  import type {AvailableVariable} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {Definition as SettingDefinition, VariableReference} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import {variableKey} from "./designerModel";
  import type {DesignerView, TemplatePart} from "./designerModel";
  import VariablePicker from "./VariablePicker.svelte";

  // Text with values inserted as pills, such as a chat message. The editor
  // owns the text once it opens, and reports every change as template parts.
  // Values are pills the caret steps over rather than {placeholders} to type,
  // so a message can't refer to a value that doesn't exist by a typo.
  let {definition, value, availableVariables, view, onChange, onDone}: {
    definition: SettingDefinition;
    value: unknown;
    availableVariables: AvailableVariable[];
    view: DesignerView;
    // Called with null when the text is cleared.
    onChange: (newValue: TemplatePart[] | null) => void;
    onDone: () => void;
  } = $props();

  // Marks a pill's element and holds its variable reference as JSON.
  const variableAttribute = "data-variable";

  let editorElement: HTMLDivElement | undefined = $state();
  let parts: TemplatePart[] = $state(untrack(() => (Array.isArray(value) ? (value as TemplatePart[]) : [])));
  // Where the caret was in the editor, so a value picked below goes there
  // even though picking it moves the focus out of the editor.
  let savedRange: Range | null = null;

  // The engine counts characters of the text only, leaving out values,
  // whose length isn't known until the automation runs.
  const characterCount = $derived(parts.reduce((count, part) => count + [...(part.text ?? "")].length, 0));
  const maximumLength = $derived(definition.maximumLength ?? 0);
  const hasMaximumLength = $derived(maximumLength > 0);
  const isTooLong = $derived(hasMaximumLength && characterCount > maximumLength);
  const isEmpty = $derived(parts.length === 0);

  function createPill(reference: VariableReference): HTMLSpanElement {
    const pill = document.createElement("span");
    pill.className = "pill";
    pill.contentEditable = "false";
    pill.setAttribute(variableAttribute, JSON.stringify(reference));
    pill.textContent = view.variableNamesByKey.get(variableKey(reference)) ?? "deleted value";
    return pill;
  }

  onMount(() => {
    if (!editorElement) {
      return;
    }
    for (const part of parts) {
      editorElement.append(part.variable ? createPill(part.variable) : document.createTextNode(part.text ?? ""));
    }
  });

  // Reads the editor's contents back into parts. The browser can wrap typed
  // or pasted text in its own elements, so anything that isn't a pill counts
  // for its text.
  function readParts(node: Node, into: TemplatePart[]): void {
    for (const child of node.childNodes) {
      if (child instanceof HTMLElement && child.hasAttribute(variableAttribute)) {
        into.push({variable: JSON.parse(child.getAttribute(variableAttribute) ?? "{}") as VariableReference});
      } else if (child instanceof HTMLElement) {
        readParts(child, into);
      } else if (child.nodeType === Node.TEXT_NODE && child.textContent) {
        const previousPart = into.at(-1);
        if (previousPart && previousPart.variable === undefined) {
          previousPart.text = (previousPart.text ?? "") + child.textContent;
        } else {
          into.push({text: child.textContent});
        }
      }
    }
  }

  function reportChange(): void {
    if (!editorElement) {
      return;
    }
    const newParts: TemplatePart[] = [];
    readParts(editorElement, newParts);
    if (newParts.length === 0) {
      // Clear what the browser leaves behind, such as a <br>, so the
      // placeholder shows again.
      editorElement.replaceChildren();
    }
    parts = newParts;
    onChange(newParts.length === 0 ? null : newParts);
  }

  function saveCaret(): void {
    const selection = window.getSelection();
    if (!editorElement || !selection || selection.rangeCount === 0) {
      return;
    }
    const range = selection.getRangeAt(0);
    if (editorElement.contains(range.commonAncestorContainer)) {
      savedRange = range.cloneRange();
    }
  }

  function placeCaret(range: Range): void {
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    savedRange = range.cloneRange();
  }

  // The first focus puts the caret at the end, ready to carry on typing.
  function placeCaretOnFirstFocus(): void {
    if (savedRange || !editorElement) {
      return;
    }
    const range = document.createRange();
    range.selectNodeContents(editorElement);
    range.collapse(false);
    placeCaret(range);
  }

  function insertAtCaret(node: Node): void {
    if (!editorElement) {
      return;
    }
    editorElement.focus({preventScroll: true});
    let range = savedRange;
    if (!range) {
      range = document.createRange();
      range.selectNodeContents(editorElement);
      range.collapse(false);
    }
    range.deleteContents();
    range.insertNode(node);
    range.setStartAfter(node);
    range.collapse(true);
    placeCaret(range);
    reportChange();
  }

  function insertVariable(reference: VariableReference): void {
    insertAtCaret(createPill(reference));
  }

  // Messages are one line, so Enter finishes instead of starting another.
  function finishOnEnter(event: KeyboardEvent): void {
    if (event.key === "Enter") {
      event.preventDefault();
      onDone();
    }
  }

  // Pasting keeps only the text, so formatting from elsewhere doesn't come
  // along, and line breaks become spaces since messages are one line.
  function pastePlainText(event: ClipboardEvent): void {
    event.preventDefault();
    const text = (event.clipboardData?.getData("text/plain") ?? "").replace(/\s*[\r\n]+\s*/g, " ");
    if (text !== "") {
      saveCaret();
      insertAtCaret(document.createTextNode(text));
    }
  }

  // Dropping text in would bring its formatting, and dragging a pill would
  // copy it as plain text.
  function preventDrop(event: DragEvent): void {
    event.preventDefault();
  }
</script>

<span class="title">{definition.displayName}</span>

<div
  class="editor field"
  class:is-too-long={isTooLong}
  class:is-empty={isEmpty}
  bind:this={editorElement}
  contenteditable="true"
  role="textbox"
  tabindex="0"
  aria-label={definition.displayName}
  aria-multiline="false"
  data-placeholder={definition.placeholder}
  data-autofocus
  oninput={reportChange}
  onkeydown={finishOnEnter}
  onkeyup={saveCaret}
  onmouseup={saveCaret}
  onblur={saveCaret}
  onfocus={placeCaretOnFirstFocus}
  onpaste={pastePlainText}
  ondrop={preventDrop}
></div>

{#if hasMaximumLength}
  <span class="count" class:is-too-long={isTooLong}>
    {characterCount} / {maximumLength}
    {#if isTooLong}· {characterCount - maximumLength} too many{/if}
  </span>
{/if}

<div class="insert">
  <span class="insert-label">Insert a value</span>
  <VariablePicker {availableVariables} {view} onPick={insertVariable}/>
</div>

<style>
  .title,
  .insert-label {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .editor {
    min-height: 2.4em;
    line-height: 1.7;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    cursor: text;
    user-select: text;
  }

  .editor.is-empty::before {
    content: attr(data-placeholder);
    color: var(--color-text-faint);
    pointer-events: none;
  }

  .editor.is-too-long {
    border-color: var(--color-danger);
    outline-color: var(--color-danger);
  }

  .editor :global(.pill) {
    padding: 0 6px;
    border-radius: var(--radius-small);
    background: var(--color-value-soft);
    color: var(--color-value);
    font-size: 0.9em;
    font-weight: 500;
    white-space: nowrap;
    user-select: all;
  }

  .count {
    align-self: flex-end;
    margin-top: calc(-1 * var(--space-1));
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    font-variant-numeric: tabular-nums;
  }

  .count.is-too-long {
    color: var(--color-danger);
  }

  .insert {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding-top: var(--space-2);
    border-top: 1px solid var(--color-border);
  }
</style>
