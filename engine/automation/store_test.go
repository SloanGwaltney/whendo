package automation_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automation"
)

func saveOrFail(t *testing.T, store *automation.Store, savedAutomation automation.Automation) {
	t.Helper()
	err := store.Save(savedAutomation)
	if err != nil {
		t.Fatalf("Save returned an error: %v", err)
	}
}

func loadAllOrFail(t *testing.T, store *automation.Store) ([]automation.Automation, []automation.LoadFailure) {
	t.Helper()
	automations, failures, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned an error: %v", err)
	}
	return automations, failures
}

// compactJSON compares automations by their encoding, because saved setting
// values are reindented in the file and so aren't byte for byte the same.
func compactJSON(t *testing.T, value any) string {
	t.Helper()
	return string(encode(t, value))
}

func writeRawFile(t *testing.T, folder string, fileName string, contents string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(folder, fileName), []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("writing %s: %v", fileName, err)
	}
}

func TestStore_LoadsWhatItSaved(t *testing.T) {
	store := automation.NewStore(filepath.Join(t.TempDir(), "automations"))
	roll := stepVariable("roll", rollVariable)
	savedAutomation := newAutomation(t,
		rollStep("roll"),
		checkStep(t, "check", roll,
			branch(automation.AfterBranchEnd, sayStep(t, "say", referenceTo(roll))),
			branch(automation.AfterBranchContinue),
		),
	)

	saveOrFail(t, store, savedAutomation)
	automations, failures := loadAllOrFail(t, store)

	if len(failures) != 0 || len(automations) != 1 {
		t.Fatalf("got %d automations and failures %v", len(automations), failures)
	}
	if compactJSON(t, automations[0]) != compactJSON(t, savedAutomation) {
		t.Errorf("loaded\n%s\nwant\n%s", compactJSON(t, automations[0]), compactJSON(t, savedAutomation))
	}
}

func TestStore_WritesFormatVersionAndNamesFileByID(t *testing.T) {
	folder := t.TempDir()
	saveOrFail(t, automation.NewStore(folder), newAutomation(t))

	contents, err := os.ReadFile(filepath.Join(folder, testAutomationID+".json"))
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}
	var file struct {
		FormatVersion int `json:"formatVersion"`
	}
	err = json.Unmarshal(contents, &file)
	if err != nil || file.FormatVersion != automation.CurrentFormatVersion {
		t.Errorf("got format version %d, error %v", file.FormatVersion, err)
	}
}

func TestStore_SaveReplacesOldVersionWithoutLeavingTemporaryFiles(t *testing.T) {
	folder := t.TempDir()
	store := automation.NewStore(folder)
	savedAutomation := newAutomation(t)
	saveOrFail(t, store, savedAutomation)

	savedAutomation.Name = "Renamed"
	saveOrFail(t, store, savedAutomation)

	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("reading the folder: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d files in the folder, want only the automation's", len(entries))
	}
	automations, _ := loadAllOrFail(t, store)
	if len(automations) != 1 || automations[0].Name != "Renamed" {
		t.Errorf("got %v, want the renamed automation", automations)
	}
}

func TestStore_LoadAllReturnsNothingForMissingFolder(t *testing.T) {
	automations, failures := loadAllOrFail(t, automation.NewStore(filepath.Join(t.TempDir(), "never-created")))

	if len(automations) != 0 || len(failures) != 0 {
		t.Errorf("got %v and %v, want nothing", automations, failures)
	}
}

func TestStore_LoadAllReportsBrokenFilesAndLoadsTheRest(t *testing.T) {
	folder := t.TempDir()
	store := automation.NewStore(folder)
	saveOrFail(t, store, newAutomation(t))
	writeRawFile(t, folder, "broken.json", `{"formatVersion": 1, "id": "broken", `)
	writeRawFile(t, folder, "notes.txt", "not an automation")

	automations, failures := loadAllOrFail(t, store)

	if len(automations) != 1 || automations[0].ID != testAutomationID {
		t.Errorf("got automations %v, want the valid one", automations)
	}
	if len(failures) != 1 || failures[0].FileName != "broken.json" {
		t.Errorf("got failures %v, want broken.json", failures)
	}
}

func TestStore_LoadAllReportsFormatProblems(t *testing.T) {
	folder := t.TempDir()
	writeRawFile(t, folder, "newer.json", `{"formatVersion": 999, "id": "newer"}`)
	writeRawFile(t, folder, "unversioned.json", `{"id": "unversioned"}`)
	writeRawFile(t, folder, "renamed.json", `{"formatVersion": 1, "id": "original"}`)

	_, failures := loadAllOrFail(t, automation.NewStore(folder))

	reasons := map[string]error{}
	for _, failure := range failures {
		reasons[failure.FileName] = failure.Reason
	}
	if !errors.Is(reasons["newer.json"], automation.ErrNewerFormat) {
		t.Errorf("newer.json: got %v, want ErrNewerFormat", reasons["newer.json"])
	}
	if !errors.Is(reasons["unversioned.json"], automation.ErrUnknownFormat) {
		t.Errorf("unversioned.json: got %v, want ErrUnknownFormat", reasons["unversioned.json"])
	}
	if reasons["renamed.json"] == nil {
		t.Error("renamed.json: a file whose name doesn't match its ID was loaded")
	}
}

func TestStore_DeleteRemovesAutomation(t *testing.T) {
	store := automation.NewStore(t.TempDir())
	saveOrFail(t, store, newAutomation(t))

	err := store.Delete(testAutomationID)

	if err != nil {
		t.Fatalf("Delete returned an error: %v", err)
	}
	automations, _ := loadAllOrFail(t, store)
	if len(automations) != 0 {
		t.Errorf("got %v after deleting, want nothing", automations)
	}
}

func TestStore_DeleteSucceedsWhenAlreadyGone(t *testing.T) {
	err := automation.NewStore(t.TempDir()).Delete(testAutomationID)

	if err != nil {
		t.Errorf("got error %v", err)
	}
}

func TestStore_RefusesIDsThatArentSafeFileNames(t *testing.T) {
	folder := t.TempDir()
	store := automation.NewStore(filepath.Join(folder, "automations"))
	unsafeAutomation := newAutomation(t)
	unsafeAutomation.ID = "../outside"

	saveErr := store.Save(unsafeAutomation)
	deleteErr := store.Delete("../outside")

	if saveErr == nil || deleteErr == nil {
		t.Errorf("got errors %v and %v, want both refused", saveErr, deleteErr)
	}
	if _, err := os.Stat(filepath.Join(folder, "outside.json")); !errors.Is(err, os.ErrNotExist) {
		t.Error("Save wrote a file outside the automations folder")
	}
}
