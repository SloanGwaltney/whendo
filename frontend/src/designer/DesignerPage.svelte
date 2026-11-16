<script lang="ts">
  import {tick} from "svelte";
  import {AutomationsService} from "../../bindings/github.com/SloanGwaltney/whendo";
  import type {CatalogDefinitions} from "../../bindings/github.com/SloanGwaltney/whendo";
  import {AfterBranch, ProblemLevel, ProblemSubject} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {Automation, Problem, Report} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {Kind} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import type {Values} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import Switch from "../Switch.svelte";
  import Toast from "../Toast.svelte";
  import {buildDesignerView, provideDesignerActions, stepElementID, triggerElementID, triggerOwnerID} from "./designerModel";
  import DesignerProblems from "./DesignerProblems.svelte";
  import {deleteStep, findStep, insertStep, moveStep, reconcileBranches, rootListKey} from "./draftEditing";
  import OptionList from "./OptionList.svelte";
  import Popover from "./Popover.svelte";
  import ReadBack from "./ReadBack.svelte";
  import SettingEditor from "./SettingEditor.svelte";
  import StepList from "./StepList.svelte";
  import StepPicker from "./StepPicker.svelte";
  import TriggerCard from "./TriggerCard.svelte";

  // The automation designer. See docs/design/mvp-ui.md#automation-designer.
  // Edits change a draft, which the engine checks as you go. Nothing is saved
  // until Save, because saved edits apply to the next run straight away.
  let {automationID, onClose, hasUnsavedChanges = $bindable(false)}: {
    automationID: string;
    onClose: () => void;
    hasUnsavedChanges?: boolean;
  } = $props();

  type DesignerMode = "build" | "readBack";

  type OpenPopover =
    | {kind: "setting"; anchor: HTMLElement; ownerID: string; settingID: string}
    | {kind: "stepPicker"; anchor: HTMLElement; listKey: string; index: number}
    | {kind: "stepMenu"; anchor: HTMLElement; stepID: string}
    | {kind: "endingMenu"; anchor: HTMLElement; stepID: string; outcomeID: string}
    | {kind: "triggerPicker"; anchor: HTMLElement};

  interface ToastMessage {
    message: string;
    canUndo: boolean;
  }

  // Waiting a moment after each edit before checking means typing checks
  // once a pause, not once a key.
  const checkDelayMilliseconds = 150;

  let catalog = $state<CatalogDefinitions | null>(null);
  let draft = $state<Automation | null>(null);
  let report = $state<Report | null>(null);
  let mode = $state<DesignerMode>("build");
  let errorMessage = $state("");
  let isSaving = $state(false);
  // DOM elements aren't plain data, so the popover state isn't made deeply
  // reactive.
  let openPopover = $state.raw<OpenPopover | null>(null);
  let toast = $state<ToastMessage | null>(null);
  // The draft before the last change that can be undone.
  let undoSnapshot = $state.raw<Automation | null>(null);

  const view = $derived(catalog && report && draft ? buildDesignerView(catalog, report, draft) : null);
  const hasMustFixProblems = $derived((report?.problems ?? []).some((problem) => problem.level === ProblemLevel.ProblemLevelMustFix));

  let checkTimer: ReturnType<typeof setTimeout> | undefined;
  let latestCheckNumber = 0;

  loadAutomation();

  async function loadAutomation(): Promise<void> {
    try {
      const [loadedCatalog, loadedAutomation] = await Promise.all([AutomationsService.Catalog(), AutomationsService.GetAutomation(automationID)]);
      catalog = loadedCatalog;
      draft = loadedAutomation;
      await checkDraft();
    } catch (error) {
      errorMessage = `Couldn't open this automation: ${error}`;
    }
  }

  // checkDraft asks the engine about the draft. Only the newest answer is
  // used, since an older one may arrive after it. When the answer shows a
  // step's paths no longer match its outcomes, the paths are fixed and the
  // draft checked again, so that answer is never shown.
  async function checkDraft(): Promise<void> {
    if (!draft) {
      return;
    }
    const checkNumber = ++latestCheckNumber;
    try {
      const newReport = await AutomationsService.CheckAutomation($state.snapshot(draft) as Automation);
      if (checkNumber !== latestCheckNumber || !draft) {
        return;
      }
      if (reconcileBranches(draft.steps, newReport)) {
        await checkDraft();
        return;
      }
      report = newReport;
    } catch (error) {
      errorMessage = `Couldn't check this automation: ${error}`;
    }
  }

  function markChanged(): void {
    hasUnsavedChanges = true;
    clearTimeout(checkTimer);
    checkTimer = setTimeout(checkDraft, checkDelayMilliseconds);
  }

  function rememberForUndo(): void {
    undoSnapshot = $state.snapshot(draft) as Automation;
  }

  function undo(): void {
    if (!undoSnapshot) {
      return;
    }
    draft = structuredClone(undoSnapshot);
    undoSnapshot = null;
    toast = null;
    openPopover = null;
    markChanged();
  }

  function showToast(message: string, canUndo = false): void {
    toast = {message, canUndo};
  }

  // Opening the popover that's already open closes it, so a second click on
  // a value acts as a toggle.
  function openOrToggle(popover: OpenPopover): void {
    openPopover = openPopover?.anchor === popover.anchor ? null : popover;
  }

  provideDesignerActions({
    editSetting: (ownerID, settingID, anchor) => openOrToggle({kind: "setting", anchor, ownerID, settingID}),
    openStepPicker: (listKey, index, anchor) => openOrToggle({kind: "stepPicker", anchor, listKey, index}),
    openStepMenu: (stepID, anchor) => openOrToggle({kind: "stepMenu", anchor, stepID}),
    openEndingMenu: (stepID, outcomeID, anchor) => openOrToggle({kind: "endingMenu", anchor, stepID, outcomeID}),
    openTriggerPicker: (anchor) => openOrToggle({kind: "triggerPicker", anchor}),
    get openAnchor() {
      return openPopover?.anchor ?? null;
    },
  });

  // The settings, definitions and available variables a setting editor
  // works on, whether the setting is the trigger's or a step's.
  function settingOwner(ownerID: string) {
    if (!draft || !view) {
      return null;
    }
    if (ownerID === triggerOwnerID) {
      draft.trigger.settings ??= {};
      return {settings: draft.trigger.settings, definitions: view.triggerDefinition?.settings ?? [], availableVariables: []};
    }
    const location = findStep(draft.steps, ownerID);
    if (!location) {
      return null;
    }
    location.step.settings ??= {};
    return {
      settings: location.step.settings,
      definitions: view.stepDefinitionsByType.get(location.step.type)?.settings ?? [],
      availableVariables: view.report.steps?.[ownerID]?.availableVariables ?? [],
    };
  }

  // The step picker and message editor need the room.
  function isWidePopover(popover: OpenPopover): boolean {
    if (popover.kind === "stepPicker") {
      return true;
    }
    if (popover.kind !== "setting") {
      return false;
    }
    const definition = settingOwner(popover.ownerID)?.definitions.find((candidate) => candidate.id === popover.settingID);
    return definition?.kind === Kind.KindTemplate;
  }

  function changeSetting(settings: NonNullable<Values>, settingID: string, newValue: unknown): void {
    if (newValue === null) {
      delete settings[settingID];
    } else {
      settings[settingID] = newValue;
    }
    markChanged();
  }

  // After adding a step or a trigger, open its first empty setting, since
  // that's the next thing to fill in.
  async function openFirstEmptySetting(elementID: string): Promise<void> {
    await tick();
    document.getElementById(elementID)?.querySelector<HTMLButtonElement>("button.value.is-empty")?.click();
  }

  async function addStep(listKey: string, index: number, stepType: string): Promise<void> {
    if (!draft) {
      return;
    }
    rememberForUndo();
    const step = insertStep(draft, listKey, index, stepType);
    openPopover = null;
    markChanged();
    if (step) {
      await openFirstEmptySetting(stepElementID(step.id));
    }
  }

  function chooseStepMenuAction(stepID: string, action: string): void {
    if (!draft || !view) {
      return;
    }
    rememberForUndo();
    openPopover = null;
    if (action === "delete") {
      const location = findStep(draft.steps, stepID);
      const name = location ? (view.stepDefinitionsByType.get(location.step.type)?.displayName ?? location.step.type) : "step";
      deleteStep(draft, stepID);
      showToast(`Deleted “${name}”.`, true);
    } else {
      moveStep(draft, stepID, action === "moveUp" ? -1 : 1);
    }
    markChanged();
  }

  function chooseEnding(stepID: string, outcomeID: string, ending: string): void {
    const branch = draft ? findStep(draft.steps, stepID)?.step.outcomes?.[outcomeID] : undefined;
    openPopover = null;
    if (!branch) {
      return;
    }
    branch.afterBranch = ending as AfterBranch;
    markChanged();
  }

  async function chooseTrigger(triggerID: string): Promise<void> {
    openPopover = null;
    if (!draft || draft.trigger.id === triggerID) {
      return;
    }
    const hadTrigger = draft.trigger.id !== "";
    rememberForUndo();
    draft.trigger = {id: triggerID, settings: {}};
    markChanged();
    if (hadTrigger) {
      showToast("Changed what starts it. Steps that used its values are listed as problems.", true);
    }
    await openFirstEmptySetting(triggerElementID);
  }

  function changeName(event: Event): void {
    if (!draft) {
      return;
    }
    draft.name = (event.currentTarget as HTMLInputElement).value;
    markChanged();
  }

  function toggleEnabled(): void {
    if (!draft) {
      return;
    }
    draft.isEnabled = !draft.isEnabled;
    markChanged();
  }

  async function save(): Promise<void> {
    if (!draft || isSaving || !hasUnsavedChanges) {
      return;
    }
    isSaving = true;
    errorMessage = "";
    try {
      const result = await AutomationsService.SaveAutomation($state.snapshot(draft) as Automation);
      hasUnsavedChanges = false;
      if (result.isTurnedOffUntilFixed) {
        draft.isEnabled = false;
        showToast("Saved, and turned off until its problems are fixed.");
      } else {
        showToast("Saved. Runs that start from now on use this version.");
      }
    } catch (error) {
      errorMessage = `Couldn't save: ${error}`;
    } finally {
      isSaving = false;
    }
  }

  $effect(() => {
    const saveOnShortcut = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        save();
      }
    };
    document.addEventListener("keydown", saveOnShortcut);
    return () => document.removeEventListener("keydown", saveOnShortcut);
  });

  // Jumping to a problem switches to Build, since Read it back has no
  // problem marks to find.
  async function jumpTo(problem: Problem): Promise<void> {
    mode = "build";
    const isAboutStep = problem.subject === ProblemSubject.ProblemSubjectStep && problem.stepId;
    const elementID = isAboutStep ? stepElementID(problem.stepId!) : triggerElementID;
    await tick();
    const element = document.getElementById(elementID);
    if (!element) {
      return;
    }
    element.scrollIntoView({block: "center", behavior: "smooth"});
    element.classList.remove("is-flashing");
    // Reading the size restarts the animation when the same step is
    // flashed twice in a row.
    void element.offsetWidth;
    element.classList.add("is-flashing");
  }

  function stepMenuOptions(stepID: string) {
    const location = draft ? findStep(draft.steps, stepID) : null;
    const index = location?.index ?? 0;
    const lastIndex = (location?.list.length ?? 1) - 1;
    const hasPaths = Object.values(location?.step.outcomes ?? {}).some((branch) => branch?.steps?.length);
    return [
      {id: "moveUp", label: "Move up", isDisabled: index === 0},
      {id: "moveDown", label: "Move down", isDisabled: index === lastIndex},
      {id: "delete", label: "Delete step", isDanger: true, description: hasPaths ? "Also deletes the steps on its paths" : undefined},
    ];
  }
</script>

<section class="designer">
  <header class="designer-header">
    <button type="button" class="back" onclick={onClose}>Home ›</button>
    {#if draft}
      <input class="name" value={draft.name} oninput={changeName} aria-label="Automation name" placeholder="Name this automation"/>
      <div class="header-actions">
        <div class="mode-tabs" role="group" aria-label="View">
          <button type="button" aria-pressed={mode === "build"} onclick={() => (mode = "build")}>Build</button>
          <button type="button" aria-pressed={mode === "readBack"} onclick={() => (mode = "readBack")}>Read it back</button>
        </div>
        <span class="enabled">
          <Switch
            isOn={draft.isEnabled}
            label={draft.isEnabled ? "Turn off" : "Turn on"}
            isDisabled={hasMustFixProblems && !draft.isEnabled}
            onToggle={toggleEnabled}
          />
          {draft.isEnabled ? "On" : "Off"}
        </span>
        <span class="save-state" class:is-unsaved={hasUnsavedChanges}>{hasUnsavedChanges ? "Unsaved changes" : "Saved"}</span>
        <button type="button" class="button is-primary" disabled={!hasUnsavedChanges || isSaving} onclick={save}>
          {isSaving ? "Saving…" : "Save"}
        </button>
      </div>
    {/if}
  </header>

  <div class="designer-body">
    {#if errorMessage}
      <p class="error is-selectable" role="alert">{errorMessage}</p>
    {/if}
    {#if draft && view}
      <DesignerProblems problems={view.report.problems ?? []} onJump={jumpTo}/>
      {#if mode === "build"}
        <div class="canvas">
          <TriggerCard trigger={draft.trigger} {view}/>
          <div>
            <div class="keyword">Do</div>
            <StepList steps={draft.steps} listKey={rootListKey} depth={0} {view}/>
          </div>
        </div>
      {:else}
        <ReadBack {draft} {view}/>
      {/if}
    {/if}
  </div>
</section>

{#if openPopover && draft && view && catalog}
  {@const popover = openPopover}
  <!-- Keyed, so each value gets a fresh editor rather than the last one's. -->
  {#key popover.anchor}
  <Popover anchor={popover.anchor} isWide={isWidePopover(popover)} onClose={() => (openPopover = null)}>
    {#if popover.kind === "setting"}
      {@const owner = settingOwner(popover.ownerID)}
      {@const definition = owner?.definitions.find((candidate) => candidate.id === popover.settingID)}
      {#if owner && definition}
        <SettingEditor
          {definition}
          value={owner.settings[popover.settingID]}
          availableVariables={owner.availableVariables}
          {view}
          onChange={(newValue) => changeSetting(owner.settings, popover.settingID, newValue)}
          onDone={() => (openPopover = null)}
        />
      {/if}
    {:else if popover.kind === "stepPicker"}
      <StepPicker definitions={catalog.steps ?? []} onChoose={(stepType) => addStep(popover.listKey, popover.index, stepType)}/>
    {:else if popover.kind === "stepMenu"}
      <OptionList options={stepMenuOptions(popover.stepID)} onChoose={(action) => chooseStepMenuAction(popover.stepID, action)}/>
    {:else if popover.kind === "endingMenu"}
      {@const currentEnding = findStep(draft.steps, popover.stepID)?.step.outcomes?.[popover.outcomeID]?.afterBranch}
      <OptionList
        options={[
          {id: AfterBranch.AfterBranchEnd, label: "Stop here", description: "The automation ends on this path.", isSelected: currentEnding === AfterBranch.AfterBranchEnd},
          {id: AfterBranch.AfterBranchContinue, label: "Carry on below", description: "Go on to the steps after this split.", isSelected: currentEnding === AfterBranch.AfterBranchContinue},
        ]}
        onChoose={(ending) => chooseEnding(popover.stepID, popover.outcomeID, ending)}
      />
    {:else if popover.kind === "triggerPicker"}
      <span class="popover-title">Start this automation when</span>
      <OptionList
        options={(catalog.triggers ?? []).map((definition) => ({id: definition.id, label: definition.displayName, description: definition.description, isSelected: definition.id === draft?.trigger.id}))}
        onChoose={chooseTrigger}
      />
    {/if}
  </Popover>
  {/key}
{/if}

{#if toast}
  <Toast message={toast.message} actionLabel={toast.canUndo ? "Undo" : ""} onAction={undo} onDismiss={() => (toast = null)}/>
{/if}

<style>
  .designer {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .designer-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3) var(--space-4);
    padding: var(--space-4) var(--space-5);
    border-bottom: 1px solid var(--color-border);
  }

  .back {
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .back:hover {
    color: var(--color-text);
  }

  .name {
    flex: 1 1 200px;
    min-width: 0;
    max-width: 420px;
    padding: 2px 6px;
    border: 1px solid transparent;
    border-radius: var(--radius);
    background: transparent;
    font-family: var(--font-display);
    font-size: var(--font-size-heading);
    font-weight: 600;
  }

  .name:hover,
  .name:focus {
    border-color: var(--color-border-strong);
    background: var(--color-surface);
    outline: none;
  }

  .header-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    margin-left: auto;
  }

  .mode-tabs {
    display: inline-flex;
    overflow: hidden;
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius);
  }

  .mode-tabs button {
    padding: 5px 12px;
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .mode-tabs button[aria-pressed="true"] {
    background: var(--color-surface-raised);
    color: var(--color-text);
  }

  .enabled {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .save-state {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
  }

  .save-state.is-unsaved {
    color: var(--color-warning);
  }

  .designer-body {
    flex: 1;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    padding: var(--space-5);
  }

  .designer-body > :global(*) {
    max-width: 900px;
  }

  .canvas {
    display: flex;
    flex-direction: column;
    gap: 22px;
  }

  .keyword {
    margin: 0 0 6px 6px;
    color: var(--color-accent);
    font-family: var(--font-display);
    font-size: 13px;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .popover-title {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .error {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius);
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }
</style>
