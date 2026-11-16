package runhistory_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/runhistory"
	"github.com/SloanGwaltney/whendo/engine/runner"
)

const concurrentWriterCount = 20

func newStore(t *testing.T) (*runhistory.Store, string) {
	t.Helper()
	folder := filepath.Join(t.TempDir(), "run-history")
	return runhistory.NewStore(folder, slog.New(slog.NewTextHandler(io.Discard, nil))), folder
}

func localTime(year int, month time.Month, day int, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, time.Local)
}

func recordEndingAt(runID string, endedAt time.Time) runner.RunRecord {
	return runner.RunRecord{
		RunID:            runID,
		AutomationID:     "check-in",
		AutomationName:   "Check In",
		TriggerVariables: map[string]string{"redeemerDisplayName": "Cooler_User"},
		QueuedAt:         endedAt.Add(-time.Second),
		StartedAt:        endedAt.Add(-time.Second),
		EndedAt:          endedAt,
		Status:           runner.RunStatusSucceeded,
		Steps:            []runner.StepRecord{{StepID: "say", StepType: "twitch.sendChatMessage", OutcomeID: "done"}},
	}
}

func writeOrFail(t *testing.T, store *runhistory.Store, record runner.RunRecord) {
	t.Helper()
	err := store.Write(record)
	if err != nil {
		t.Fatalf("Write returned an error: %v", err)
	}
}

func readDayOrFail(t *testing.T, store *runhistory.Store, day string) []runner.RunRecord {
	t.Helper()
	records, err := store.ReadDay(day)
	if err != nil {
		t.Fatalf("ReadDay returned an error: %v", err)
	}
	return records
}

func runIDs(records []runner.RunRecord) []string {
	var ids []string
	for _, record := range records {
		ids = append(ids, record.RunID)
	}
	return ids
}

func TestStore_ReadDayReturnsRunsNewestFirst(t *testing.T) {
	store, _ := newStore(t)
	writeOrFail(t, store, recordEndingAt("morning", localTime(2026, time.November, 15, 9)))
	writeOrFail(t, store, recordEndingAt("evening", localTime(2026, time.November, 15, 21)))

	records := readDayOrFail(t, store, "2026-11-15")

	if !slices.Equal(runIDs(records), []string{"evening", "morning"}) {
		t.Errorf("got runs %v, want evening then morning", runIDs(records))
	}
	if records[1].TriggerVariables["redeemerDisplayName"] != "Cooler_User" || records[1].Steps[0].StepID != "say" {
		t.Errorf("got record %+v, want every field kept", records[1])
	}
}

func TestStore_FilesRunsByTheLocalDayTheyEnded(t *testing.T) {
	store, folder := newStore(t)
	writeOrFail(t, store, recordEndingAt("lateNight", localTime(2026, time.November, 14, 23)))
	writeOrFail(t, store, recordEndingAt("nextMorning", localTime(2026, time.November, 15, 1)))

	for _, fileName := range []string{"2026-11-14.jsonl", "2026-11-15.jsonl"} {
		if _, err := os.Stat(filepath.Join(folder, fileName)); err != nil {
			t.Errorf("%s: %v", fileName, err)
		}
	}
	if records := readDayOrFail(t, store, "2026-11-14"); !slices.Equal(runIDs(records), []string{"lateNight"}) {
		t.Errorf("got runs %v for 2026-11-14", runIDs(records))
	}
}

func TestStore_DaysListsDaysNewestFirstAndIgnoresOtherFiles(t *testing.T) {
	store, folder := newStore(t)
	writeOrFail(t, store, recordEndingAt("older", localTime(2026, time.October, 31, 12)))
	writeOrFail(t, store, recordEndingAt("newer", localTime(2026, time.November, 15, 12)))
	for _, strayName := range []string{"notes.txt", "not-a-day.jsonl"} {
		err := os.WriteFile(filepath.Join(folder, strayName), nil, 0o600)
		if err != nil {
			t.Fatalf("writing %s: %v", strayName, err)
		}
	}

	days, err := store.Days()

	if err != nil || !slices.Equal(days, []string{"2026-11-15", "2026-10-31"}) {
		t.Errorf("got days %v, error %v", days, err)
	}
}

func TestStore_ReturnsNothingBeforeAnyRunIsWritten(t *testing.T) {
	store, _ := newStore(t)

	days, daysErr := store.Days()
	records, readErr := store.ReadDay("2026-11-15")

	if daysErr != nil || readErr != nil || len(days) != 0 || len(records) != 0 {
		t.Errorf("got days %v (%v) and records %v (%v), want nothing", days, daysErr, records, readErr)
	}
}

func TestStore_ReadDayRefusesDaysNotInDayFormat(t *testing.T) {
	store, _ := newStore(t)

	for _, badDay := range []string{"../secrets", "2026-13-01", "15/11/2026"} {
		_, err := store.ReadDay(badDay)
		if !errors.Is(err, runhistory.ErrInvalidDay) {
			t.Errorf("%q: got error %v, want ErrInvalidDay", badDay, err)
		}
	}
}

func TestStore_SkipsUnreadableLinesAndKeepsTheRest(t *testing.T) {
	store, folder := newStore(t)
	writeOrFail(t, store, recordEndingAt("before", localTime(2026, time.November, 15, 9)))
	appendRaw(t, filepath.Join(folder, "2026-11-15.jsonl"), "{not json}\n")
	writeOrFail(t, store, recordEndingAt("after", localTime(2026, time.November, 15, 10)))

	records := readDayOrFail(t, store, "2026-11-15")

	if !slices.Equal(runIDs(records), []string{"after", "before"}) {
		t.Errorf("got runs %v, want both readable runs", runIDs(records))
	}
}

func TestStore_LineCutShortByCrashDoesntSpoilNextRun(t *testing.T) {
	store, folder := newStore(t)
	appendRaw(t, filepath.Join(folder, "2026-11-15.jsonl"), `{"runId":"cut sho`)

	writeOrFail(t, store, recordEndingAt("afterCrash", localTime(2026, time.November, 15, 10)))

	if records := readDayOrFail(t, store, "2026-11-15"); !slices.Equal(runIDs(records), []string{"afterCrash"}) {
		t.Errorf("got runs %v, want the run written after the crash", runIDs(records))
	}
}

func appendRaw(t *testing.T, filePath string, contents string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(filePath), 0o700)
	if err != nil {
		t.Fatalf("creating folder: %v", err)
	}
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("opening %s: %v", filePath, err)
	}
	_, writeErr := file.WriteString(contents)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("writing %s: %v %v", filePath, writeErr, closeErr)
	}
}

func TestStore_ConcurrentWritesKeepEveryRun(t *testing.T) {
	store, _ := newStore(t)
	endedAt := localTime(2026, time.November, 15, 12)
	var writers sync.WaitGroup
	writeErrors := make(chan error, concurrentWriterCount)

	for writerIndex := range concurrentWriterCount {
		writers.Go(func() {
			writeErrors <- store.Write(recordEndingAt(fmt.Sprintf("run-%d", writerIndex), endedAt))
		})
	}
	writers.Wait()
	close(writeErrors)

	for err := range writeErrors {
		if err != nil {
			t.Errorf("Write returned an error: %v", err)
		}
	}
	if records := readDayOrFail(t, store, "2026-11-15"); len(records) != concurrentWriterCount {
		t.Errorf("got %d runs, want %d", len(records), concurrentWriterCount)
	}
}

func TestStore_PruneDeletesOnlyDaysPastRetention(t *testing.T) {
	store, _ := newStore(t)
	now := localTime(2026, time.November, 15, 12)
	writeOrFail(t, store, recordEndingAt("tooOld", localTime(2026, time.October, 15, 12)))
	writeOrFail(t, store, recordEndingAt("oldestKept", localTime(2026, time.October, 16, 12)))
	writeOrFail(t, store, recordEndingAt("today", now))

	err := store.Prune(runhistory.DefaultRetentionDays, now)

	if err != nil {
		t.Fatalf("Prune returned an error: %v", err)
	}
	days, err := store.Days()
	if err != nil || !slices.Equal(days, []string{"2026-11-15", "2026-10-16"}) {
		t.Errorf("got days %v, error %v; want only October 15 deleted", days, err)
	}
}

func TestStore_KeepPrunedPrunesRightAway(t *testing.T) {
	store, _ := newStore(t)
	writeOrFail(t, store, recordEndingAt("ancient", time.Now().AddDate(-1, 0, 0)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store.KeepPruned(ctx, func() int { return runhistory.DefaultRetentionDays })

	if days, err := store.Days(); err != nil || len(days) != 0 {
		t.Errorf("got days %v, error %v; want the old day pruned", days, err)
	}
}
