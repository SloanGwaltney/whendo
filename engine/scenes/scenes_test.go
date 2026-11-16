package scenes_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/scenes"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

const testProgramName = "Streamlabs Desktop"

// fakeConnection is a program's connection holding a fixed set of scenes.
type fakeConnection struct {
	isConnected      bool
	scenes           []scenes.Scene
	visibilityError  error
	mutex            sync.Mutex
	visibilityCalls  []visibilityCall
	allScenesFetches int
}

type visibilityCall struct {
	itemID    string
	isVisible bool
}

func (connection *fakeConnection) IsConnected() bool {
	return connection.isConnected
}

func (connection *fakeConnection) Scene(ctx context.Context, sceneID string) (scenes.Scene, bool, error) {
	for _, scene := range connection.scenes {
		if scene.ID == sceneID {
			return scene, true, nil
		}
	}
	return scenes.Scene{}, false, nil
}

func (connection *fakeConnection) Scenes(ctx context.Context) ([]scenes.Scene, error) {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	connection.allScenesFetches++
	return connection.scenes, nil
}

func (connection *fakeConnection) SetItemVisibility(ctx context.Context, item scenes.Item, isVisible bool) error {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	if connection.visibilityError != nil {
		return connection.visibilityError
	}
	connection.visibilityCalls = append(connection.visibilityCalls, visibilityCall{itemID: item.ID, isVisible: isVisible})
	return nil
}

// fakeProgram is the streamer's choice: a connection, or none.
type fakeProgram struct {
	connection *fakeConnection
}

func (program fakeProgram) Chosen() (scenes.ChosenProgram, bool) {
	if program.connection == nil {
		return scenes.ChosenProgram{}, false
	}
	return scenes.ChosenProgram{Name: testProgramName, Connection: program.connection}, true
}

// gameplayScene has Head Pets on both of Streamlabs' canvases, inside a
// folder, plus another source.
func gameplayScene() scenes.Scene {
	return scenes.Scene{
		ID:   "scene_gameplay",
		Name: "Gameplay",
		Items: []scenes.Item{
			{ID: "item_horizontal", SourceID: "source_head_pets", SourceName: "Head Pets", FolderPath: []string{"Alerts"}, IsVisible: false},
			{ID: "item_vertical", SourceID: "source_head_pets", SourceName: "Head Pets", FolderPath: []string{"Alerts"}, IsVisible: true},
			{ID: "item_webcam", SourceID: "source_webcam", SourceName: "Webcam", IsVisible: true},
		},
	}
}

func connectedProgram(sceneList ...scenes.Scene) (fakeProgram, *fakeConnection) {
	connection := &fakeConnection{isConnected: true, scenes: sceneList}
	return fakeProgram{connection: connection}, connection
}

func headPetsSource() scenes.SourceInScene {
	return scenes.SourceInScene{SceneID: "scene_gameplay", SceneName: "Gameplay", SourceID: "source_head_pets", SourceName: "Head Pets"}
}

func sourceSettings(t *testing.T, source any) automationsetting.Values {
	t.Helper()
	encodedSource, err := json.Marshal(source)
	if err != nil {
		t.Fatalf("encoding the source: %v", err)
	}
	return automationsetting.Values{scenes.SourceSetting: encodedSource}
}

// findStep returns one of the steps by ID.
func findStep(t *testing.T, program scenes.Program, known *scenes.KnownScenes, stepID string) step.Step {
	t.Helper()
	for _, visibilityStep := range scenes.Steps(program, known) {
		if visibilityStep.Definition().ID == stepID {
			return visibilityStep
		}
	}
	t.Fatalf("no step %q", stepID)
	return nil
}

func runStep(t *testing.T, program scenes.Program, stepID string, source any) error {
	t.Helper()
	visibilityStep := findStep(t, program, &scenes.KnownScenes{}, stepID)
	result, err := visibilityStep.Run(context.Background(), step.Input{Settings: sourceSettings(t, source)})
	if err == nil && result.OutcomeID != step.DefaultOutcomeID {
		t.Errorf("outcome = %q, want %q", result.OutcomeID, step.DefaultOutcomeID)
	}
	return err
}

func noteMessages(t *testing.T, program scenes.Program, known *scenes.KnownScenes, source any) []string {
	t.Helper()
	noter, canNote := findStep(t, program, known, scenes.ShowSourceStepID).(step.Noter)
	if !canNote {
		t.Fatal("the show step doesn't note how it's used")
	}
	var messages []string
	for _, note := range noter.NoteStep(step.Usage{Settings: sourceSettings(t, source)}) {
		messages = append(messages, note.Message)
	}
	return messages
}

func TestSteps_AreAcceptedByTheCatalog(t *testing.T) {
	program, _ := connectedProgram()

	_, err := automation.NewCatalog(twitcheventsub.Triggers(), scenes.Steps(program, &scenes.KnownScenes{}))

	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
}

func TestShowSource_ShowsEveryHiddenItemOfTheSourceInTheScene(t *testing.T) {
	program, connection := connectedProgram(gameplayScene())

	err := runStep(t, program, scenes.ShowSourceStepID, headPetsSource())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []visibilityCall{{itemID: "item_horizontal", isVisible: true}}
	if len(connection.visibilityCalls) != 1 || connection.visibilityCalls[0] != want[0] {
		t.Fatalf("visibility calls = %+v, want %+v", connection.visibilityCalls, want)
	}
}

func TestHideSource_HidesEveryVisibleItemOfTheSourceInTheScene(t *testing.T) {
	program, connection := connectedProgram(gameplayScene())

	err := runStep(t, program, scenes.HideSourceStepID, headPetsSource())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := visibilityCall{itemID: "item_vertical", isVisible: false}
	if len(connection.visibilityCalls) != 1 || connection.visibilityCalls[0] != want {
		t.Fatalf("visibility calls = %+v, want %+v", connection.visibilityCalls, want)
	}
}

func TestShowSource_SucceedsWithoutChangesWhenAlreadyShowing(t *testing.T) {
	program, connection := connectedProgram(gameplayScene())
	webcam := scenes.SourceInScene{SceneID: "scene_gameplay", SceneName: "Gameplay", SourceID: "source_webcam", SourceName: "Webcam"}

	err := runStep(t, program, scenes.ShowSourceStepID, webcam)

	if err != nil || len(connection.visibilityCalls) != 0 {
		t.Fatalf("Run = %v with calls %+v, want success and no calls", err, connection.visibilityCalls)
	}
}

func TestShowSource_FindsTheSceneByNameWhenItsIDIsGone(t *testing.T) {
	recreatedScene := gameplayScene()
	recreatedScene.ID = "scene_recreated"
	program, connection := connectedProgram(recreatedScene)

	err := runStep(t, program, scenes.ShowSourceStepID, headPetsSource())

	if err != nil || len(connection.visibilityCalls) != 1 {
		t.Fatalf("Run = %v with calls %+v, want Head Pets shown", err, connection.visibilityCalls)
	}
	if connection.allScenesFetches != 1 {
		t.Errorf("fetched every scene %d times, want once", connection.allScenesFetches)
	}
}

func TestShowSource_FindsTheSourceByNameWhenItsIDIsGone(t *testing.T) {
	scene := gameplayScene()
	scene.Items = []scenes.Item{{ID: "item_recreated", SourceID: "source_recreated", SourceName: "Head Pets"}}
	program, connection := connectedProgram(scene)

	err := runStep(t, program, scenes.ShowSourceStepID, headPetsSource())

	if err != nil || len(connection.visibilityCalls) != 1 || connection.visibilityCalls[0].itemID != "item_recreated" {
		t.Fatalf("Run = %v with calls %+v, want the recreated item shown", err, connection.visibilityCalls)
	}
	if connection.allScenesFetches != 0 {
		t.Errorf("fetched every scene %d times, want never, since the scene's ID was found", connection.allScenesFetches)
	}
}

func TestShowSource_Fails(t *testing.T) {
	sceneWithoutHeadPets := gameplayScene()
	sceneWithoutHeadPets.Items = sceneWithoutHeadPets.Items[2:]
	testCases := []struct {
		name        string
		program     fakeProgram
		wantMessage string
	}{
		{"when no program is chosen", fakeProgram{}, "no streaming software is chosen"},
		{"when the program isn't connected", fakeProgram{connection: &fakeConnection{}}, "Streamlabs Desktop isn't connected"},
		{"when the scene is gone", fakeProgram{connection: &fakeConnection{isConnected: true}}, "the “Gameplay” scene isn't in Streamlabs Desktop any more"},
		{"when the source is gone", fakeProgram{connection: &fakeConnection{isConnected: true, scenes: []scenes.Scene{sceneWithoutHeadPets}}}, "“Head Pets” isn't in “Gameplay” any more"},
		{"when the program refuses", fakeProgram{connection: &fakeConnection{isConnected: true, scenes: []scenes.Scene{gameplayScene()}, visibilityError: errors.New("Streamlabs Desktop didn't answer")}}, "couldn't show “Head Pets”: Streamlabs Desktop didn't answer"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := runStep(t, testCase.program, scenes.ShowSourceStepID, headPetsSource())

			if err == nil || !strings.Contains(err.Error(), testCase.wantMessage) {
				t.Fatalf("error = %v, want one containing %q", err, testCase.wantMessage)
			}
		})
	}
}

func TestShowSource_NotesWhatWouldMakeItFail(t *testing.T) {
	sceneWithoutHeadPets := gameplayScene()
	sceneWithoutHeadPets.Items = sceneWithoutHeadPets.Items[2:]
	knownWithoutHeadPets := &scenes.KnownScenes{}
	knownWithoutHeadPets.Remember([]scenes.Scene{sceneWithoutHeadPets})
	knownWithHeadPets := &scenes.KnownScenes{}
	knownWithHeadPets.Remember([]scenes.Scene{gameplayScene()})
	connected, _ := connectedProgram()
	testCases := []struct {
		name        string
		program     fakeProgram
		known       *scenes.KnownScenes
		wantMessage string
	}{
		{"no program chosen", fakeProgram{}, &scenes.KnownScenes{}, "Choose your streaming software in Settings, so this step has something to show and hide."},
		{"the program not connected", fakeProgram{connection: &fakeConnection{}}, knownWithHeadPets, "Streamlabs Desktop isn't connected, so this step will fail until it is."},
		{"the source gone", connected, knownWithoutHeadPets, "“Head Pets” isn't in “Gameplay” any more. Pick the source again."},
		{"nothing when no list is known", connected, &scenes.KnownScenes{}, ""},
		{"nothing when the source is there", connected, knownWithHeadPets, ""},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			messages := noteMessages(t, testCase.program, testCase.known, headPetsSource())

			if testCase.wantMessage == "" {
				if len(messages) != 0 {
					t.Fatalf("notes = %q, want none", messages)
				}
				return
			}
			if len(messages) != 1 || messages[0] != testCase.wantMessage {
				t.Fatalf("notes = %q, want %q", messages, testCase.wantMessage)
			}
		})
	}
}

func TestKnownScenes_ForgettingStopsSourcesBeingCalledMissing(t *testing.T) {
	known := &scenes.KnownScenes{}
	known.Remember([]scenes.Scene{})
	connected, _ := connectedProgram()

	known.Forget()

	if messages := noteMessages(t, connected, known, headPetsSource()); len(messages) != 0 {
		t.Fatalf("notes = %q after forgetting, want none", messages)
	}
}

func TestShowSource_RefusesASourceThatCantBeRead(t *testing.T) {
	program, _ := connectedProgram()
	checker, canCheck := findStep(t, program, &scenes.KnownScenes{}, scenes.ShowSourceStepID).(step.UsageChecker)
	if !canCheck {
		t.Fatal("the show step doesn't check how it's used")
	}

	for _, unreadableSource := range []any{"Head Pets", map[string]string{"sceneId": "scene_gameplay"}} {
		problems := checker.CheckUsage(step.Usage{Settings: sourceSettings(t, unreadableSource)})

		if len(problems) != 1 || problems[0].SettingID != scenes.SourceSetting {
			t.Errorf("problems for %v = %+v, want one about the source", unreadableSource, problems)
		}
	}
	if problems := checker.CheckUsage(step.Usage{Settings: sourceSettings(t, headPetsSource())}); len(problems) != 0 {
		t.Errorf("problems for a readable source = %+v, want none", problems)
	}
}
