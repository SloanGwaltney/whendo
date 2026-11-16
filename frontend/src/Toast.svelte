<script lang="ts">
  // A short message at the bottom of the window that goes away by itself,
  // with an optional action such as Undo.
  let {message, actionLabel = "", onAction, onDismiss}: {
    message: string;
    actionLabel?: string;
    onAction?: () => void;
    onDismiss: () => void;
  } = $props();

  const visibleMilliseconds = 6000;

  // Restart the timer whenever the message changes, so a new toast gets its
  // full time.
  $effect(() => {
    void message;
    const timer = setTimeout(onDismiss, visibleMilliseconds);
    return () => clearTimeout(timer);
  });
</script>

<div class="toast" role="status">
  <span>{message}</span>
  {#if actionLabel && onAction}
    <button type="button" onclick={onAction}>{actionLabel}</button>
  {/if}
</div>

<style>
  .toast {
    position: fixed;
    left: 50%;
    bottom: var(--space-5);
    z-index: 70;
    display: flex;
    align-items: center;
    gap: 14px;
    max-width: calc(100vw - 32px);
    padding: 9px 14px;
    transform: translateX(-50%);
    border-radius: var(--radius-large);
    background: var(--color-text);
    color: var(--color-background);
    font-weight: 500;
    box-shadow: 0 10px 30px var(--color-shadow);
  }

  button {
    font-weight: 700;
    text-decoration: underline;
    text-underline-offset: 3px;
  }
</style>
