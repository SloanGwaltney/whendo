// Helpers for the Run History page. See docs/design/mvp-ui.md#run-history.
import {RunStatus} from "../bindings/github.com/SloanGwaltney/whendo/engine/runner";
import type {RunRecord, StepRecord} from "../bindings/github.com/SloanGwaltney/whendo/engine/runner";

const millisecondsPerSecond = 1000;
const secondsPerMinute = 60;
// Go writes a time that was never set, such as when a run that never left
// the queue started, as the year 1.
const latestUnsetYear = 1;

export const statusLabels: Record<RunStatus, string> = {
  [RunStatus.$zero]: "Unknown",
  [RunStatus.RunStatusSucceeded]: "Succeeded",
  [RunStatus.RunStatusFailed]: "Failed",
  [RunStatus.RunStatusCancelled]: "Cancelled",
};

export function isTimeSet(time: string): boolean {
  return new Date(time).getFullYear() > latestUnsetYear;
}

// dayOfRun returns the day a run is filed under, the day it ended in local
// time, written like the engine's file names: "2026-11-15".
export function dayOfRun(record: RunRecord): string {
  return formatDay(new Date(record.endedAt));
}

function formatDay(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const dayOfMonth = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${month}-${dayOfMonth}`;
}

// describeDay names a day the way a streamer would: "Today", "Yesterday",
// or the date.
export function describeDay(day: string, now: Date): string {
  if (day === formatDay(now)) {
    return "Today";
  }
  // Going back a calendar day rather than 24 hours stays right on the days
  // clocks change.
  if (day === formatDay(new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1))) {
    return "Yesterday";
  }
  const [year, month, dayOfMonth] = day.split("-").map(Number);
  const date = new Date(year, month - 1, dayOfMonth);
  const isThisYear = year === now.getFullYear();
  return date.toLocaleDateString(undefined, {weekday: "long", day: "numeric", month: "long", year: isThisYear ? undefined : "numeric"});
}

export function formatTime(time: string): string {
  return new Date(time).toLocaleTimeString(undefined, {timeStyle: "medium"});
}

export function formatElapsed(startTime: string, endTime: string): string {
  const elapsedSeconds = (new Date(endTime).getTime() - new Date(startTime).getTime()) / millisecondsPerSecond;
  if (elapsedSeconds < 1) {
    return "less than a second";
  }
  const wholeSeconds = Math.round(elapsedSeconds);
  if (wholeSeconds < secondsPerMinute) {
    return pluralize(wholeSeconds, "second");
  }
  const minutes = Math.floor(wholeSeconds / secondsPerMinute);
  const remainingSeconds = wholeSeconds % secondsPerMinute;
  return remainingSeconds === 0 ? pluralize(minutes, "minute") : `${pluralize(minutes, "minute")} ${pluralize(remainingSeconds, "second")}`;
}

function pluralize(count: number, unit: string): string {
  return `${count} ${unit}${count === 1 ? "" : "s"}`;
}

export function failedStep(record: RunRecord): StepRecord | undefined {
  return record.steps?.find((stepRecord) => stepRecord.failureReason);
}

// describeProblem says why a run didn't succeed, naming the step that
// failed when one did. It's empty for runs that succeeded.
export function describeProblem(record: RunRecord): string {
  if (record.status === RunStatus.RunStatusSucceeded) {
    return "";
  }
  const stepRecord = failedStep(record);
  if (!stepRecord) {
    return record.failureReason ?? "";
  }
  const stepNumber = (record.steps ?? []).indexOf(stepRecord) + 1;
  return `Step ${stepNumber}, ${stepName(stepRecord)}: ${stepRecord.failureReason}`;
}

// stepName falls back to the step's type for records written before steps
// kept their names.
export function stepName(stepRecord: StepRecord): string {
  return stepRecord.stepName || stepRecord.stepType;
}

// mergeRuns combines a day read from its file with runs that ended while
// the page was open, newest first. A run can be in both, because it's
// written to its file before the page hears it ended.
export function mergeRuns(loadedRuns: RunRecord[], liveRuns: RunRecord[]): RunRecord[] {
  const loadedRunIDs = new Set(loadedRuns.map((record) => record.runId));
  const newRuns = liveRuns.filter((record) => !loadedRunIDs.has(record.runId));
  return [...newRuns, ...loadedRuns].sort((first, second) => new Date(second.endedAt).getTime() - new Date(first.endedAt).getTime());
}
