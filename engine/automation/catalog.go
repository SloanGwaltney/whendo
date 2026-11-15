package automation

import (
	"errors"
	"fmt"

	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// ErrDuplicateID means two triggers or two steps given to a catalog share an
// ID.
var ErrDuplicateID = errors.New("duplicate ID")

// Catalog holds every trigger and step Whendo offers, so automations can be
// checked and run by looking them up by ID.
type Catalog struct {
	triggersByID map[string]trigger.Trigger
	stepsByID    map[string]step.Step
}

// NewCatalog returns a catalog of the given triggers and steps.
func NewCatalog(triggers []trigger.Trigger, steps []step.Step) (*Catalog, error) {
	catalog := &Catalog{
		triggersByID: make(map[string]trigger.Trigger, len(triggers)),
		stepsByID:    make(map[string]step.Step, len(steps)),
	}
	for _, catalogTrigger := range triggers {
		triggerID := catalogTrigger.Definition().ID
		if _, isTaken := catalog.triggersByID[triggerID]; isTaken {
			return nil, fmt.Errorf("%w: two triggers have ID %q", ErrDuplicateID, triggerID)
		}
		catalog.triggersByID[triggerID] = catalogTrigger
	}
	for _, catalogStep := range steps {
		stepID := catalogStep.Definition().ID
		if _, isTaken := catalog.stepsByID[stepID]; isTaken {
			return nil, fmt.Errorf("%w: two steps have ID %q", ErrDuplicateID, stepID)
		}
		catalog.stepsByID[stepID] = catalogStep
	}
	return catalog, nil
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
