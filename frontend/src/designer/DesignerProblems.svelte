<script lang="ts">
  import {ProblemLevel} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {Problem} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";

  // The problems list above the steps. See docs/design/mvp-ui.md#problems.
  let {problems, onJump}: {problems: Problem[]; onJump: (problem: Problem) => void} = $props();

  const mustFixCount = $derived(problems.filter((problem) => problem.level === ProblemLevel.ProblemLevelMustFix).length);
  const goodToKnowCount = $derived(problems.length - mustFixCount);
</script>

{#if problems.length > 0}
  <details class="problems" open>
    <summary>
      {#if mustFixCount > 0}<span class="count is-must-fix">{mustFixCount} to fix</span>{/if}
      {#if goodToKnowCount > 0}<span class="count is-good-to-know">{goodToKnowCount} to know</span>{/if}
      <span class="summary-text">{mustFixCount > 0 ? "This automation stays off until these are fixed." : "It can run, but these will matter."}</span>
    </summary>
    <ul>
      {#each problems as problem, problemIndex (problemIndex)}
        <li>
          <button type="button" onclick={() => onJump(problem)}>
            <span class="mark" class:is-must-fix={problem.level === ProblemLevel.ProblemLevelMustFix}>
              {problem.level === ProblemLevel.ProblemLevelMustFix ? "!" : "i"}
            </span>
            <span>{problem.message}</span>
          </button>
        </li>
      {/each}
    </ul>
  </details>
{/if}

<style>
  .problems {
    border: 1px solid var(--color-border);
    border-radius: var(--radius-large);
    background: var(--color-surface);
  }

  summary {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    cursor: pointer;
    font-weight: 500;
    list-style: none;
  }

  summary::-webkit-details-marker {
    display: none;
  }

  .count {
    padding: 1px var(--space-2);
    border-radius: 999px;
    font-size: var(--font-size-small);
    font-weight: 600;
  }

  .count.is-must-fix {
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }

  .count.is-good-to-know {
    background: var(--color-warning-soft);
    color: var(--color-warning);
  }

  .summary-text {
    color: var(--color-text-muted);
    font-size: 13px;
    font-weight: 400;
  }

  ul {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin: 0;
    padding: 0 var(--space-2) var(--space-2);
    list-style: none;
  }

  li button {
    display: flex;
    gap: 10px;
    width: 100%;
    padding: 6px var(--space-2);
    border-radius: var(--radius);
    color: var(--color-text-muted);
    text-align: left;
  }

  li button:hover {
    background: var(--color-surface-raised);
    color: var(--color-text);
  }

  .mark {
    flex: none;
    width: 16px;
    color: var(--color-warning);
    font-weight: 700;
    text-align: center;
  }

  .mark.is-must-fix {
    color: var(--color-danger);
  }
</style>
