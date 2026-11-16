// The streaming software the streamer chose and its connection status,
// shared by the navigation and the Settings page. It follows the engine's
// changes for as long as the app runs. See
// docs/design/streaming-software.md.
import {Events} from "@wailsio/runtime";
import {StreamingSoftwareStateChanged} from "../bindings/events";
import {StreamingSoftwareService} from "../bindings/github.com/SloanGwaltney/whendo";
import type {StreamingSoftwareState} from "../bindings/github.com/SloanGwaltney/whendo";
import {StreamingSoftware} from "../bindings/github.com/SloanGwaltney/whendo/engine/appsettings";
import {ConnectionStatus} from "../bindings/github.com/SloanGwaltney/whendo/engine/streamlabs";

// Every program the streamer can choose, in the order Settings lists them.
export const streamingSoftwareChoices: {software: StreamingSoftware; label: string}[] = [
  {software: StreamingSoftware.StreamingSoftwareNone, label: "None"},
  {software: StreamingSoftware.StreamingSoftwareStreamlabsDesktop, label: "Streamlabs Desktop"},
];

interface TrackedState {
  // The latest state, or null before the first has loaded.
  current: StreamingSoftwareState | null;
}

export const streamingSoftwareState: TrackedState = $state({current: null});

// Subscribe before asking for the current state, so a change that happens
// in between isn't missed.
Events.On(StreamingSoftwareStateChanged, (event) => {
  streamingSoftwareState.current = event.data;
});
StreamingSoftwareService.GetState()
  .then((currentState) => {
    streamingSoftwareState.current ??= currentState;
  })
  .catch((error: unknown) => console.error("Couldn't load the streaming software state", error));

export function isStreamingSoftwareChosen(state: StreamingSoftwareState | null): boolean {
  return state !== null && state.software !== StreamingSoftware.StreamingSoftwareNone;
}

export function isStreamingSoftwareConnected(state: StreamingSoftwareState | null): boolean {
  return isStreamingSoftwareChosen(state) && state?.status === ConnectionStatus.ConnectionStatusConnected;
}

export function chooseStreamingSoftware(software: StreamingSoftware): Promise<void> {
  return StreamingSoftwareService.ChooseStreamingSoftware(software);
}
