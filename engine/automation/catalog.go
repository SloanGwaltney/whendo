package automation

import (
	"errors"
	"fmt"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

var (
	// ErrDuplicateID means two triggers or two steps given to a catalog
	// share an ID.
	ErrDuplicateID = errors.New("duplicate ID")
	// ErrSentenceMismatch means a trigger's or step's sentence doesn't place
	// each of its settings exactly once, so the designer couldn't show it.
	ErrSentenceMismatch = errors.New("sentence doesn't match settings")
)

// Catalog holds every trigger and step Whendo offers, so automations can be
// checked and run by looking them up by ID. It keeps them in the order they
// were given, which is the order the designer lists them in.
type Catalog struct {
	triggers     []trigger.Trigger
	steps        []step.Step
	triggersByID map[string]trigger.Trigger
	stepsByID    map[string]step.Step
}

// NewCatalog returns a catalog of the given triggers and steps. It returns
// an error for a definition the designer couldn't show, so a mistake in one
// is caught when Whendo starts rather than when someone opens the designer.
func NewCatalog(triggers []trigger.Trigger, steps []step.Step) (*Catalog, error) {
	catalog := &Catalog{
		triggers:     triggers,
		steps:        steps,
		triggersByID: make(map[string]trigger.Trigger, len(triggers)),
		stepsByID:    make(map[string]step.Step, len(steps)),
	}
	for _, catalogTrigger := range triggers {
		definition := catalogTrigger.Definition()
		if _, isTaken := catalog.triggersByID[definition.ID]; isTaken {
			return nil, fmt.Errorf("%w: two triggers have ID %q", ErrDuplicateID, definition.ID)
		}
		err := checkSentence(definition.Sentence, definition.Settings)
		if err != nil {
			return nil, fmt.Errorf("trigger %q: %w", definition.ID, err)
		}
		catalog.triggersByID[definition.ID] = catalogTrigger
	}
	for _, catalogStep := range steps {
		definition := catalogStep.Definition()
		if _, isTaken := catalog.stepsByID[definition.ID]; isTaken {
			return nil, fmt.Errorf("%w: two steps have ID %q", ErrDuplicateID, definition.ID)
		}
		err := checkSentence(definition.Sentence, definition.Settings)
		if err != nil {
			return nil, fmt.Errorf("step %q: %w", definition.ID, err)
		}
		catalog.stepsByID[definition.ID] = catalogStep
	}
	return catalog, nil
}

// checkSentence makes sure every setting can be edited in the designer,
// which edits settings only through their place in the sentence.
func checkSentence(sentence automationsetting.Sentence, settings []automationsetting.Definition) error {
	placedSettingIDs := sentence.SettingIDs()
	for _, setting := range settings {
		placementCount := 0
		for _, placedSettingID := range placedSettingIDs {
			if placedSettingID == setting.ID {
				placementCount++
			}
		}
		if placementCount != 1 {
			return fmt.Errorf("%w: setting %q appears %d times", ErrSentenceMismatch, setting.ID, placementCount)
		}
	}
	if len(placedSettingIDs) != len(settings) {
		return fmt.Errorf("%w: the sentence places a setting that isn't defined", ErrSentenceMismatch)
	}
	return nil
}

// TriggerDefinitions returns the definition of every trigger, in order.
func (catalog *Catalog) TriggerDefinitions() []trigger.Definition {
	definitions := make([]trigger.Definition, 0, len(catalog.triggers))
	for _, catalogTrigger := range catalog.triggers {
		definitions = append(definitions, catalogTrigger.Definition())
	}
	return definitions
}

// StepDefinitions returns the definition of every step, in order.
func (catalog *Catalog) StepDefinitions() []step.Definition {
	definitions := make([]step.Definition, 0, len(catalog.steps))
	for _, catalogStep := range catalog.steps {
		definitions = append(definitions, catalogStep.Definition())
	}
	return definitions
}

// Trigger returns the trigger with the given ID.
func (catalog *Catalog) Trigger(triggerID string) (trigger.Trigger, bool) {
	catalogTrigger, isFound := catalog.triggersByID[triggerID]
	return catalogTrigger, isFound
}

// Step returns the step with the given ID.
func (catalog *Catalog) Step(stepID string) (step.Step, bool) {
	catalogStep, isFound := catalog.stepsByID[stepID]
	return catalogStep, isFound
}
