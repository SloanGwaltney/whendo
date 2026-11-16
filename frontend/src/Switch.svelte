<script lang="ts">
  let {isOn, label, isDisabled = false, onToggle}: {
    isOn: boolean;
    // Read by screen readers, since the switch shows no text of its own.
    label: string;
    isDisabled?: boolean;
    onToggle: () => void;
  } = $props();
</script>

<button
  type="button"
  class="switch"
  role="switch"
  aria-checked={isOn}
  aria-label={label}
  disabled={isDisabled}
  onclick={onToggle}
>
  <span class="track"></span>
</button>

<style>
  .switch {
    display: inline-flex;
    padding: 2px;
    border-radius: 999px;
  }

  .switch:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .track {
    position: relative;
    width: 34px;
    height: 20px;
    border-radius: 999px;
    background: var(--color-border-strong);
    transition: background 0.15s;
  }

  .track::after {
    content: "";
    position: absolute;
    top: 3px;
    left: 3px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: var(--color-text);
    transition: transform 0.15s;
  }

  .switch[aria-checked="true"] .track {
    background: var(--color-success);
  }

  .switch[aria-checked="true"] .track::after {
    transform: translateX(14px);
  }
</style>
