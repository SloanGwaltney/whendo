import {AfterBranch} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
import type {Automation, Report, Step} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";

// Changes to a draft automation in the designer. Each function changes the
// draft in place, so Svelte's deep reactivity updates the page.

// A list of steps is either the automation's own, or one path of a step that
// splits, named "stepID/outcomeID".
export const rootListKey = "root";

export function branchListKey(stepID: string, outcomeID: string): string {
  return `${stepID}/${outcomeID}`;
}

export interface StepLocation {
  list: Step[];
  index: number;
  step: Step;
}

export function findStep(steps: Step[] | null, stepID: string): StepLocation | null {
  for (const [index, step] of (steps ?? []).entries()) {
    if (step.id === stepID) {
      return {list: steps!, index, step};
    }
    for (const branch of Object.values(step.outcomes ?? {})) {
      const found = findStep(branch?.steps ?? null, stepID);
      if (found) {
        return found;
      }
    }
  }
  return null;
}

// stepListByKey returns the list a key names, creating it if a path has
// none yet, or null if the step it names is gone.
export function stepListByKey(draft: Automation, listKey: string): Step[] | null {
  if (listKey === rootListKey) {
    draft.steps ??= [];
    return draft.steps;
  }
  const [stepID, outcomeID] = listKey.split("/");
  const location = findStep(draft.steps, stepID);
  const branch = location?.step.outcomes?.[outcomeID];
  if (!branch) {
    return null;
  }
  branch.steps ??= [];
  return branch.steps;
}

// newStepID only needs to be unique within the automation; see
// docs/design/automation-engine.md#saving-automations.
export function newStepID(): string {
  return crypto.randomUUID();
}

export function insertStep(draft: Automation, listKey: string, index: number, stepType: string): Step | null {
  const list = stepListByKey(draft, listKey);
  if (!list) {
    return null;
  }
  const step: Step = {id: newStepID(), type: stepType, settings: {}};
  list.splice(index, 0, step);
  return step;
}

export function deleteStep(draft: Automation, stepID: string): void {
  const location = findStep(draft.steps, stepID);
  location?.list.splice(location.index, 1);
}

// moveStep moves a step one place within its own list. Steps never move
// into or out of a path; see docs/design/mvp-ui.md#automation-designer.
export function moveStep(draft: Automation, stepID: string, offset: -1 | 1): void {
  const location = findStep(draft.steps, stepID);
  if (!location) {
    return;
  }
  const targetIndex = location.index + offset;
  if (targetIndex < 0 || targetIndex >= location.list.length) {
    return;
  }
  const [moved] = location.list.splice(location.index, 1);
  location.list.splice(targetIndex, 0, moved);
}

// reconcileBranches gives every step that splits exactly one path per
// outcome, using the outcomes the engine reported, since outcomes can
// depend on settings. New paths carry on below, as decided in
// docs/design/mvp-ui.md#automation-designer. A path for an outcome that's
// gone is removed only when it's empty, so steps are never lost silently;
// one that still has steps stays for the engine to report. It returns
// whether anything changed.
export function reconcileBranches(steps: Step[] | null, report: Report): boolean {
  let hasChanged = false;
  for (const step of steps ?? []) {
    const outcomes = report.steps?.[step.id]?.outcomes ?? [];
    if (outcomes.length > 1) {
      step.outcomes ??= {};
      const outcomeIDs = new Set(outcomes.map((outcome) => outcome.id));
      for (const outcomeID of outcomeIDs) {
        if (!step.outcomes[outcomeID]) {
          step.outcomes[outcomeID] = {steps: [], afterBranch: AfterBranch.AfterBranchContinue};
          hasChanged = true;
        }
      }
      for (const [branchID, branch] of Object.entries(step.outcomes)) {
        if (!outcomeIDs.has(branchID) && !branch?.steps?.length) {
          delete step.outcomes[branchID];
          hasChanged = true;
        }
      }
    }
    for (const branch of Object.values(step.outcomes ?? {})) {
      hasChanged = reconcileBranches(branch?.steps ?? null, report) || hasChanged;
    }
  }
  return hasChanged;
}
