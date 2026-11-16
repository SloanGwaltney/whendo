<script lang="ts">
  import type {Snippet} from "svelte";

  // A small panel under the value being edited, which keeps the sentence in
  // view. See docs/design/mvp-ui.md#automation-designer.
  let {anchor, isWide = false, onClose, children}: {
    anchor: HTMLElement;
    // Wider for the step picker and message editor, which need the room.
    isWide?: boolean;
    onClose: () => void;
    children: Snippet;
  } = $props();

  // Space kept between the popover and the anchor or the window's edges.
  const gapPixels = 8;
  const edgeMarginPixels = 12;
  // The tallest a popover gets even with room to spare, so a long list
  // doesn't cover the whole sentence.
  const maximumHeightPixels = 520;
  const maximumWindowHeightShare = 0.7;

  let popoverElement: HTMLDivElement | undefined = $state();
  let left = $state(0);
  let top = $state(0);
  let maxHeight = $state(maximumHeightPixels);

  // Follow the anchor every frame while open, since editing re-renders the
  // sentence around it and the page can scroll underneath. Close if the
  // anchor leaves the page, such as when its step is deleted.
  $effect(() => {
    let frameRequest = 0;
    const place = () => {
      if (!anchor.isConnected) {
        onClose();
        return;
      }
      if (popoverElement) {
        placeAround(popoverElement, anchor.getBoundingClientRect());
      }
      frameRequest = requestAnimationFrame(place);
    };
    place();
    return () => cancelAnimationFrame(frameRequest);
  });

  // placeAround puts the popover below the anchor, or above when only that
  // fits. When neither fits, it takes the side with more room and limits
  // its height to that room, so its contents scroll instead of running off
  // the window. It measures the contents' full height rather than the
  // popover's, which the limit itself would shrink, so the choice can't
  // flip back and forth between frames.
  function placeAround(element: HTMLDivElement, anchorBox: DOMRect): void {
    const width = element.offsetWidth;
    const borderHeight = element.offsetHeight - element.clientHeight;
    const tallestAllowed = Math.min(maximumHeightPixels, window.innerHeight * maximumWindowHeightShare);
    const height = Math.min(element.scrollHeight + borderHeight, tallestAllowed);
    left = Math.max(edgeMarginPixels, Math.min(anchorBox.left, window.innerWidth - width - edgeMarginPixels));
    const roomBelow = window.innerHeight - edgeMarginPixels - anchorBox.bottom - gapPixels;
    const roomAbove = anchorBox.top - gapPixels - edgeMarginPixels;
    const isBelow = height <= roomBelow || (height > roomAbove && roomBelow >= roomAbove);
    maxHeight = Math.min(tallestAllowed, isBelow ? roomBelow : roomAbove);
    const shownHeight = Math.min(height, maxHeight);
    top = isBelow ? anchorBox.bottom + gapPixels : anchorBox.top - gapPixels - shownHeight;
  }

  // Close on a press outside the popover. A press on the anchor itself is
  // left to the anchor, which closes the popover by toggling it.
  $effect(() => {
    const closeOnOutsidePress = (event: PointerEvent) => {
      const target = event.target as Node;
      if (popoverElement?.contains(target) || anchor.contains(target)) {
        return;
      }
      onClose();
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") {
        return;
      }
      onClose();
      anchor.focus();
    };
    document.addEventListener("pointerdown", closeOnOutsidePress, true);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePress, true);
      document.removeEventListener("keydown", closeOnEscape);
    };
  });

  // Put the keyboard in the popover, on whatever asks for it or else its
  // first control.
  $effect(() => {
    const focusTarget = popoverElement?.querySelector<HTMLElement>("[data-autofocus]") ?? popoverElement?.querySelector<HTMLElement>("input, select, button");
    focusTarget?.focus({preventScroll: true});
  });
</script>

<div class="popover" class:is-wide={isWide} bind:this={popoverElement} style:left="{left}px" style:top="{top}px" style:max-height="{maxHeight}px">
  {@render children()}
</div>

<style>
  .popover {
    position: fixed;
    z-index: 50;
    display: flex;
    flex-direction: column;
    gap: 10px;
    width: 340px;
    max-width: calc(100vw - 24px);
    overflow: auto;
    padding: var(--space-3);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-large);
    background: var(--color-surface-raised);
    box-shadow: 0 14px 40px var(--color-shadow);
  }

  .popover.is-wide {
    width: 440px;
  }

  /* Whendo assumes at least an HD monitor, so wider windows get a roomier
     editor; see docs/design/mvp-ui.md#automation-designer. */
  @media (min-width: 1600px) {
    .popover.is-wide {
      width: 560px;
    }
  }
</style>
