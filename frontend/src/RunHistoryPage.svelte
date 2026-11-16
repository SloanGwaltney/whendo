<script lang="ts">
  import {Events} from "@wailsio/runtime";
  import {RunEnded} from "../bindings/events";
  import {AutomationsService} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {CatalogDefinitions} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {RunRecord} from "../bindings/github.com/SloanGwaltney/whendo/engine/runner";
  import Page from "./Page.svelte";
  import RunDetailsDialog from "./RunDetailsDialog.svelte";
  import {dayOfRun, describeDay, describeProblem, formatTime, mergeRuns} from "./runHistory";
  import RunStatusBadge from "./RunStatusBadge.svelte";

  // Every run that has ended, a day at a time, newest first. Runs still
  // queued or running aren't shown yet. See docs/design/mvp-ui.md#run-history.

  interface LoadedDay {
    day: string;
    runs: RunRecord[];
  }

  let catalog = $state<CatalogDefinitions | null>(null);
  // Days with run history, newest first.
  let days = $state<string[]>([]);
  let selectedDay = $state("");
  let loadedDay = $state<LoadedDay | null>(null);
  // Runs that ended while the page was open, by day, newest first. They're
  // merged with what's read from the files, so a run that ends while a day
  // is loading isn't lost.
  let liveRunsByDay = $state<Record<string, RunRecord[]>>({});
  let hasLoadedDays = $state(false);
  let errorMessage = $state("");
  let openRecord = $state<RunRecord | null>(null);

  const now = new Date();
  const selectedDayIndex = $derived(days.indexOf(selectedDay));
  const shownRuns = $derived(mergeRuns(loadedDay?.day === selectedDay ? loadedDay.runs : [], liveRunsByDay[selectedDay] ?? []));
  const isLoadingSelectedDay = $derived(selectedDay !== "" && loadedDay?.day !== selectedDay);
  const openTriggerDefinition = $derived((catalog?.triggers ?? []).find((definition) => definition.id === openRecord?.triggerId) ?? null);

  $effect(() => {
    // Subscribe before loading, so a run that ends in between is kept.
    return Events.On(RunEnded, (event) => addEndedRun(event.data));
  });

  loadPage();

  async function loadPage(): Promise<void> {
    try {
      const historyDays = (await AutomationsService.RunHistoryDays()) ?? [];
      days = mergeDays(days, historyDays);
      hasLoadedDays = true;
      if (selectedDay === "" && days.length > 0) {
        await selectDay(days[0]);
      }
    } catch (error) {
      errorMessage = `Couldn't load run history: ${error}`;
    }
    // The catalog only names triggers and their values in a run's details,
    // so history still shows without it.
    try {
      catalog = await AutomationsService.Catalog();
    } catch (error) {
      console.error("Couldn't load the trigger and step catalog", error);
    }
  }

  async function selectDay(day: string): Promise<void> {
    selectedDay = day;
    errorMessage = "";
    try {
      const runs = (await AutomationsService.RunHistory(day)) ?? [];
      // Another day may have been picked while this one loaded.
      if (selectedDay === day) {
        loadedDay = {day, runs};
      }
    } catch (error) {
      errorMessage = `Couldn't load the runs for ${describeDay(day, now)}: ${error}`;
    }
  }

  function addEndedRun(record: RunRecord): void {
    const day = dayOfRun(record);
    liveRunsByDay = {...liveRunsByDay, [day]: [record, ...(liveRunsByDay[day] ?? [])]};
    days = mergeDays(days, [day]);
    if (selectedDay === "") {
      selectDay(day);
    }
  }

  function mergeDays(knownDays: string[], addedDays: string[]): string[] {
    // Days written as YYYY-MM-DD sort by date when sorted as text.
    return [...new Set([...knownDays, ...addedDays])].sort().reverse();
  }
</script>

<Page title="Run History">
  <div class="run-history">
    {#if days.length > 0}
      <div class="toolbar">
        <button type="button" class="button" disabled={selectedDayIndex <= 0} onclick={() => selectDay(days[selectedDayIndex - 1])}>Newer</button>
        <select class="field day-picker" aria-label="Day" value={selectedDay} onchange={(event) => selectDay(event.currentTarget.value)}>
          {#each days as day (day)}
            <option value={day}>{describeDay(day, now)}</option>
          {/each}
        </select>
        <button type="button" class="button" disabled={selectedDayIndex >= days.length - 1} onclick={() => selectDay(days[selectedDayIndex + 1])}>Older</button>
      </div>
    {/if}

    {#if errorMessage}
      <p class="error is-selectable" role="alert">{errorMessage}</p>
    {/if}

    {#if hasLoadedDays && days.length === 0}
      <div class="empty">
        <p>Nothing has run yet.</p>
        <p class="hint">Each time one of your automations runs, it's listed here with how it went.</p>
      </div>
    {:else if shownRuns.length > 0}
      <table>
        <thead>
          <tr>
            <th scope="col">Automation</th>
            <th scope="col">Triggered</th>
            <th scope="col">Status</th>
            <th scope="col">What went wrong</th>
          </tr>
        </thead>
        <tbody>
          {#each shownRuns as record (record.runId)}
            <tr onclick={() => (openRecord = record)}>
              <td>
                <!-- The row opens on a click anywhere; the button makes it
                     reachable from the keyboard. -->
                <button type="button" class="automation-name" onclick={(event) => { event.stopPropagation(); openRecord = record; }}>{record.automationName}</button>
              </td>
              <td class="time">{formatTime(record.queuedAt)}</td>
              <td><RunStatusBadge status={record.status} isTest={record.isTest}/></td>
              <td class="problem">{describeProblem(record)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {:else if !isLoadingSelectedDay && !errorMessage}
      <p class="hint">No runs on this day.</p>
    {/if}
  </div>
</Page>

<RunDetailsDialog record={openRecord} triggerDefinition={openTriggerDefinition} onClose={() => (openRecord = null)}/>

<style>
  .run-history {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 1080px;
  }

  .toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .day-picker {
    width: auto;
    min-width: 220px;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-large);
    background: var(--color-surface);
    overflow: hidden;
  }

  th {
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--color-border);
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-align: left;
    text-transform: uppercase;
  }

  td {
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--color-border);
    vertical-align: baseline;
  }

  tbody tr:last-child td {
    border-bottom: 0;
  }

  tbody tr {
    cursor: pointer;
  }

  tbody tr:hover {
    background: var(--color-accent-soft);
  }

  .automation-name {
    font-weight: 600;
    text-align: left;
    overflow-wrap: anywhere;
  }

  tbody tr:hover .automation-name {
    color: var(--color-accent);
  }

  .time {
    white-space: nowrap;
    color: var(--color-text-muted);
    font-variant-numeric: tabular-nums;
  }

  .problem {
    color: var(--color-text-muted);
    overflow-wrap: anywhere;
  }

  .empty {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-6) var(--space-5);
    border: 1px dashed var(--color-border-strong);
    border-radius: var(--radius-large);
    text-align: center;
  }

  .hint {
    color: var(--color-text-muted);
  }

  .error {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius);
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }
</style>
