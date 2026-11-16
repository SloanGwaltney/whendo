import type {CatalogDefinitions} from "../../bindings/github.com/SloanGwaltney/whendo";
import {ProblemLevel, ProblemSubject} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
import type {Automation, AvailableVariable, Report} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
import {Kind, VariableSource} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
import type {VariableReference} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
import type {Definition as StepDefinition} from "../../bindings/github.com/SloanGwaltney/whendo/engine/step";
import type {Definition as TriggerDefinition} from "../../bindings/github.com/SloanGwaltney/whendo/engine/trigger";

// The engine stores these inside settings' raw JSON, so the bindings don't
// generate them. They mirror automationsetting.TemplatePart and Choice.
export interface TemplatePart {
  text?: string;
  variable?: VariableReference;
}

export interface Choice {
  id: string;
  text: string;
}

// What the designer's components need to show an automation, worked out
// once from the catalog and the engine's report on the draft.
export interface DesignerView {
  triggerDefinition: TriggerDefinition | null;
  stepDefinitionsByType: Map<string, StepDefinition>;
  report: Report;
  // Display names of every variable anything in the automation provides,
  // keyed by variableKey, so a value used where it isn't available can
  // still be named.
  variableNamesByKey: Map<string, string>;
  // The worst problem level of each step, for marking its number.
  problemLevelsByStepID: Map<string, ProblemLevel>;
  // Settings with a problem, keyed by settingKey.
  settingsWithProblems: Set<string>;
}

// The owner of the trigger's settings in settingKey, since it has no step ID.
export const triggerOwnerID = "trigger";

export function variableKey(reference: VariableReference): string {
  return `${reference.source}|${reference.stepId ?? ""}|${reference.id}`;
}

export function settingKey(ownerID: string, settingID: string): string {
  return `${ownerID}:${settingID}`;
}

export function buildDesignerView(catalog: CatalogDefinitions, report: Report, draft: Automation): DesignerView {
  const triggerDefinition = (catalog.triggers ?? []).find((definition) => definition.id === draft.trigger.id) ?? null;
  const variableNamesByKey = new Map<string, string>();
  const rememberNames = (variables: AvailableVariable[] | null) => {
    for (const variable of variables ?? []) {
      variableNamesByKey.set(variableKey(variable.reference), variable.displayName);
    }
  };
  for (const [stepID, stepReport] of Object.entries(report.steps ?? {})) {
    if (!stepReport) {
      continue;
    }
    rememberNames(stepReport.availableVariables);
    for (const outcome of stepReport.outcomes ?? []) {
      for (const variable of outcome.variables ?? []) {
        variableNamesByKey.set(variableKey({source: VariableSource.VariableSourceStep, stepId: stepID, id: variable.id}), variable.displayName);
      }
    }
  }
  for (const variable of triggerDefinition?.variables ?? []) {
    variableNamesByKey.set(variableKey({source: VariableSource.VariableSourceTrigger, id: variable.id}), variable.displayName);
  }

  const problemLevelsByStepID = new Map<string, ProblemLevel>();
  const settingsWithProblems = new Set<string>();
  for (const problem of report.problems ?? []) {
    if (problem.stepId && problemLevelsByStepID.get(problem.stepId) !== ProblemLevel.ProblemLevelMustFix) {
      problemLevelsByStepID.set(problem.stepId, problem.level);
    }
    if (problem.settingId) {
      const ownerID = problem.subject === ProblemSubject.ProblemSubjectTrigger ? triggerOwnerID : (problem.stepId ?? "");
      settingsWithProblems.add(settingKey(ownerID, problem.settingId));
    }
  }

  return {
    triggerDefinition,
    stepDefinitionsByType: new Map((catalog.steps ?? []).map((definition) => [definition.id, definition])),
    report,
    variableNamesByKey,
    problemLevelsByStepID,
    settingsWithProblems,
  };
}

const millisecondsPerSecond = 1000;
const secondsPerMinute = 60;

export function formatDuration(milliseconds: number): string {
  const totalSeconds = Math.round(milliseconds / millisecondsPerSecond);
  const minutes = Math.floor(totalSeconds / secondsPerMinute);
  const seconds = totalSeconds % secondsPerMinute;
  const parts: string[] = [];
  if (minutes > 0) {
    parts.push(`${minutes} ${minutes === 1 ? "minute" : "minutes"}`);
  }
  if (seconds > 0 || minutes === 0) {
    parts.push(`${seconds} ${seconds === 1 ? "second" : "seconds"}`);
  }
  return parts.join(" ");
}

// isEmptyValue matches the engine's idea of a missing value, so the designer
// shows the placeholder exactly when the engine would ask for one.
export function isEmptyValue(kind: Kind, value: unknown): boolean {
  if (value === undefined || value === null) {
    return true;
  }
  switch (kind) {
    case Kind.KindText:
      return String(value).trim() === "";
    case Kind.KindTemplate:
      return (value as TemplatePart[]).every((part) => !part.variable && !(part.text ?? "").trim());
    case Kind.KindChoiceList:
      return (value as Choice[]).length === 0;
    default:
      return false;
  }
}

// The page element of a step, which problems jump to.
export function stepElementID(stepID: string): string {
  return `designer-step-${stepID}`;
}

export const triggerElementID = "designer-trigger";
