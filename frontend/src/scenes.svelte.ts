// The chosen streaming software's scenes, shared by the source picker and
// the sentences that name a source. It's loaded when first needed and on
// refresh, not kept in sync with the program. See
// docs/design/streaming-software.md#picking-a-source.
import {StreamingSoftwareService} from "../bindings/github.com/SloanGwaltney/whendo";
import type {Kind} from "../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
import type {Scene} from "../bindings/github.com/SloanGwaltney/whendo/engine/scenes";

// This mirrors scenes.SourceSettingKind, which the bindings don't generate
// because it's a Go constant rather than an enum.
export const sourceSettingKind = "scenes.sceneSource" as Kind;

// SourceInScene mirrors scenes.SourceInScene, the value of a source
// setting. The bindings don't generate it, since no bound method uses it.
export interface SourceInScene {
  sceneId: string;
  sceneName: string;
  sourceId: string;
  sourceName: string;
}

interface ScenesState {
  // The last list that loaded, or null before one has.
  list: Scene[] | null;
  isLoading: boolean;
  errorMessage: string;
}

export const scenesState: ScenesState = $state({list: null, isLoading: false, errorMessage: ""});

export async function loadScenes(): Promise<void> {
  if (scenesState.isLoading) {
    return;
  }
  scenesState.isLoading = true;
  scenesState.errorMessage = "";
  try {
    scenesState.list = await StreamingSoftwareService.ListScenes();
  } catch (error) {
    scenesState.errorMessage = `Couldn't load your scenes: ${error}`;
  } finally {
    scenesState.isLoading = false;
  }
}

export function loadScenesIfNeeded(): void {
  if (scenesState.list === null && !scenesState.isLoading) {
    loadScenes();
  }
}

// describeSource names a source for a sentence, such as “Head Pets in
// Gameplay”. Names come from the last list when it has the IDs, so a
// renamed scene or source shows its new name, and otherwise from the
// setting, which keeps the names it was picked with.
export function describeSource(value: unknown): string {
  const source = value as SourceInScene;
  const scene = scenesState.list?.find((listedScene) => listedScene.id === source.sceneId);
  const item = scene?.items?.find((listedItem) => listedItem.sourceId === source.sourceId);
  return `${item?.sourceName ?? source.sourceName} in ${scene?.name ?? source.sceneName}`;
}
