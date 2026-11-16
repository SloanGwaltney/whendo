package scenes

import (
	"context"
	"errors"
	"fmt"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
)

const (
	// ShowSourceStepID is the step that makes a source visible in a scene.
	ShowSourceStepID = "scenes.showSource"
	// HideSourceStepID is the step that makes a source invisible in a scene.
	HideSourceStepID = "scenes.hideSource"
	// SourceSetting is the source in a scene the steps show or hide.
	SourceSetting = "source"

	// scenesGroup is the step picker's group for steps that act on scenes.
	scenesGroup = "Scenes"

	notChosenFailure = "no streaming software is chosen, so choose it in Settings"
	notChosenNote    = "Choose your streaming software in Settings, so this step has something to show and hide."
	// unreadableSourceProblem is a must-fix problem, since only a
	// hand-edited file can cause it.
	unreadableSourceProblem = "This step's source can't be read. Pick the source again."
)

// visibilityWording is what differs between showing and hiding.
type visibilityWording struct {
	stepID      string
	displayName string
	description string
	// verb words the sentence and failures, such as "Show" and "show".
	sentenceVerb string
	verb         string
	isVisible    bool
}

var showWording = visibilityWording{
	stepID:      ShowSourceStepID,
	displayName: "Show a source",
	// Hiding and showing a browser source doesn't restart it without this
	// option, which Head Pets needs. See the design's "Head Pets and
	// Browser Sources".
	description:  "Show a source in a scene, like clicking its eye icon. Tip: for a browser source to start over each time it's shown, tick “Shutdown source when not visible” in its properties.",
	sentenceVerb: "Show",
	verb:         "show",
	isVisible:    true,
}

var hideWording = visibilityWording{
	stepID:       HideSourceStepID,
	displayName:  "Hide a source",
	description:  "Hide a source in a scene, like clicking its eye icon.",
	sentenceVerb: "Hide",
	verb:         "hide",
	isVisible:    false,
}

// Steps returns the show and hide steps, acting on the chosen program. They
// use known to point out a source that's gone.
func Steps(program Program, known *KnownScenes) []step.Step {
	return []step.Step{
		visibilityStep{program: program, known: known, wording: showWording},
		visibilityStep{program: program, known: known, wording: hideWording},
	}
}

// visibilityStep shows or hides a source in a scene. Showing and hiding
// differ only in the visibility they set and how they're worded. See
// docs/design/streaming-software.md#show-and-hide-a-source.
type visibilityStep struct {
	program Program
	known   *KnownScenes
	wording visibilityWording
}

func (visibility visibilityStep) Definition() step.Definition {
	return step.Definition{
		ID:          visibility.wording.stepID,
		DisplayName: visibility.wording.displayName,
		Description: visibility.wording.description,
		Group:       scenesGroup,
		Sentence:    automationsetting.Sentence{automationsetting.Words(visibility.wording.sentenceVerb), automationsetting.Setting(SourceSetting)},
		Settings: []automationsetting.Definition{
			{ID: SourceSetting, DisplayName: "Source", Kind: SourceSettingKind, IsRequired: true, Placeholder: "pick a source"},
		},
	}
}

func (visibilityStep) Outcomes(automationsetting.Values) ([]step.Outcome, error) {
	return []step.Outcome{{ID: step.DefaultOutcomeID, DisplayName: "Done"}}, nil
}

// CheckUsage refuses a source setting that can't be read.
func (visibilityStep) CheckUsage(usage step.Usage) []step.Note {
	_, err := ReadSource(usage.Settings, SourceSetting)
	if errors.Is(err, ErrUnreadableSource) {
		return []step.Note{{SettingID: SourceSetting, Message: unreadableSourceProblem}}
	}
	return nil
}

// NoteStep points out what would make the step fail that isn't a mistake
// in the automation: no program chosen, the program not connected, or the
// source gone, as far as the last list of scenes says.
func (visibility visibilityStep) NoteStep(usage step.Usage) []step.Note {
	chosen, isChosen := visibility.program.Chosen()
	if !isChosen {
		return []step.Note{{SettingID: SourceSetting, Message: notChosenNote}}
	}
	if !chosen.Connection.IsConnected() {
		return []step.Note{{SettingID: SourceSetting, Message: fmt.Sprintf("%s isn't connected, so this step will fail until it is.", chosen.Name)}}
	}
	source, err := ReadSource(usage.Settings, SourceSetting)
	if err != nil || !visibility.known.isMissing(source) {
		return nil
	}
	return []step.Note{{SettingID: SourceSetting, Message: fmt.Sprintf("“%s” isn't in “%s” any more. Pick the source again.", source.SourceName, source.SceneName)}}
}

// Run shows or hides every item in the scene that shows the source. One
// already as asked is left alone: the streamer asked for an end state, and
// it's already true.
func (visibility visibilityStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	source, err := ReadSource(input.Settings, SourceSetting)
	if err != nil {
		return step.Result{}, fmt.Errorf("reading the source: %w", err)
	}
	chosen, isChosen := visibility.program.Chosen()
	if !isChosen {
		return step.Result{}, errors.New(notChosenFailure)
	}
	if !chosen.Connection.IsConnected() {
		return step.Result{}, fmt.Errorf("couldn't %s “%s” because %s isn't connected", visibility.wording.verb, source.SourceName, chosen.Name)
	}
	scene, err := visibility.findScene(ctx, chosen, source)
	if err != nil {
		return step.Result{}, err
	}
	items := FindItems(scene, source)
	if len(items) == 0 {
		return step.Result{}, fmt.Errorf("“%s” isn't in “%s” any more", source.SourceName, scene.Name)
	}
	for _, item := range items {
		if item.IsVisible == visibility.wording.isVisible {
			continue
		}
		err = chosen.Connection.SetItemVisibility(ctx, item, visibility.wording.isVisible)
		if err != nil {
			return step.Result{}, fmt.Errorf("couldn't %s “%s”: %w", visibility.wording.verb, source.SourceName, err)
		}
	}
	return step.Result{OutcomeID: step.DefaultOutcomeID}, nil
}

// findScene asks for the source's scene by ID, and looks for it by name
// among every scene if the ID isn't found.
func (visibility visibilityStep) findScene(ctx context.Context, chosen ChosenProgram, source SourceInScene) (Scene, error) {
	if source.SceneID != "" {
		scene, isFound, err := chosen.Connection.Scene(ctx, source.SceneID)
		if err != nil {
			return Scene{}, fmt.Errorf("couldn't get the “%s” scene: %w", source.SceneName, err)
		}
		if isFound {
			return scene, nil
		}
	}
	allScenes, err := chosen.Connection.Scenes(ctx)
	if err != nil {
		return Scene{}, fmt.Errorf("couldn't get the scenes: %w", err)
	}
	scene, isFound := findScene(allScenes, source)
	if !isFound {
		return Scene{}, fmt.Errorf("the “%s” scene isn't in %s any more", source.SceneName, chosen.Name)
	}
	return scene, nil
}
