<script lang="ts">
  import type {Snippet} from "svelte";

  // A modal dialog on the browser's own <dialog>, which already traps focus
  // and closes on Escape.
  let {title, isOpen, onClose, isWide = false, children}: {
    title: string;
    isOpen: boolean;
    // Called however the dialog closes, including Escape, so the caller's
    // isOpen stays in step.
    onClose: () => void;
    // Wide dialogs show details rather than ask a question.
    isWide?: boolean;
    children: Snippet;
  } = $props();

  const titleID = $props.id();
  let dialogElement: HTMLDialogElement | undefined = $state();

  $effect(() => {
    if (!dialogElement) {
      return;
    }
    if (isOpen && !dialogElement.open) {
      dialogElement.showModal();
    } else if (!isOpen && dialogElement.open) {
      dialogElement.close();
    }
  });
</script>

<dialog bind:this={dialogElement} class:is-wide={isWide} aria-labelledby={titleID} onclose={onClose}>
  <h2 id={titleID}>{title}</h2>
  {@render children()}
</dialog>

<style>
  dialog {
    width: min(440px, calc(100vw - 32px));
    padding: var(--space-5);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-large);
    background: var(--color-surface-raised);
    color: var(--color-text);
    box-shadow: 0 14px 40px var(--color-shadow);
  }

  dialog.is-wide {
    width: min(640px, calc(100vw - 32px));
  }

  dialog[open] {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  dialog::backdrop {
    background: var(--color-shadow);
  }

  h2 {
    font-size: 20px;
  }
</style>
