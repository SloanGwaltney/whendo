package twitchrewards_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/twitchapi"
	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
)

// changedRewardJSON is Twitch's answer after creating or changing a reward.
const changedRewardJSON = `{"data":[{
	"id": "new-reward", "title": "Hydrate", "prompt": "Drink water", "cost": 100,
	"background_color": "#00C7AC", "is_enabled": true, "is_paused": false, "is_user_input_required": false,
	"should_redemptions_skip_request_queue": true,
	"max_per_stream_setting": {"is_enabled": false, "max_per_stream": 0},
	"max_per_user_per_stream_setting": {"is_enabled": true, "max_per_user_per_stream": 2},
	"global_cooldown_setting": {"is_enabled": true, "global_cooldown_seconds": 60}
}]}`

// recordedRequest is what the fake Twitch API saw of one request.
type recordedRequest struct {
	method string
	query  url.Values
	body   map[string]any
}

// recordingAPI answers every request the same way and records them.
type recordingAPI struct {
	t          *testing.T
	statusCode int
	body       string
	mutex      sync.Mutex
	requests   []recordedRequest
}

func (api *recordingAPI) handle(writer http.ResponseWriter, request *http.Request) {
	api.mutex.Lock()
	defer api.mutex.Unlock()
	encodedBody, err := io.ReadAll(request.Body)
	if err != nil {
		api.t.Errorf("reading the request body: %v", err)
	}
	var body map[string]any
	if len(encodedBody) > 0 {
		err = json.Unmarshal(encodedBody, &body)
		if err != nil {
			api.t.Errorf("decoding the request body: %v", err)
		}
	}
	api.requests = append(api.requests, recordedRequest{method: request.Method, query: request.URL.Query(), body: body})
	writer.WriteHeader(api.statusCode)
	_, err = writer.Write([]byte(api.body))
	if err != nil {
		api.t.Errorf("writing the response: %v", err)
	}
}

func (api *recordingAPI) received() []recordedRequest {
	api.mutex.Lock()
	defer api.mutex.Unlock()
	return api.requests
}

func newRecordingSource(t *testing.T, statusCode int, body string) (*twitchrewards.TwitchSource, *recordingAPI) {
	t.Helper()
	api := &recordingAPI{t: t, statusCode: statusCode, body: body}
	server := httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(server.Close)
	client := twitchapi.NewClient(twitchapi.Config{ClientID: testClientID, Account: fakeAccount{isLoggedIn: true}, BaseURL: server.URL, HTTPClient: server.Client()})
	return twitchrewards.NewTwitchSource(client), api
}

func hydrateSettings() twitchrewards.Settings {
	settings := twitchrewards.NewSettings()
	settings.Title = "Hydrate"
	settings.Prompt = "Drink water"
	settings.Cost = 100
	settings.MaximumPerViewerPerStream = 2
	settings.CooldownSeconds = 60
	return settings
}

func TestSettingsValidate_EnforcesTwitchsRules(t *testing.T) {
	longestTitle := strings.Repeat("é", twitchrewards.MaximumTitleLength)
	cases := map[string]struct {
		change  func(settings *twitchrewards.Settings)
		isValid bool
	}{
		"a valid redeem":                  {change: func(*twitchrewards.Settings) {}, isValid: true},
		"the longest name, in characters": {change: func(settings *twitchrewards.Settings) { settings.Title = longestTitle }, isValid: true},
		"a blank name":                    {change: func(settings *twitchrewards.Settings) { settings.Title = "  " }},
		"a name one character too long":   {change: func(settings *twitchrewards.Settings) { settings.Title = longestTitle + "é" }},
		"a cost of zero":                  {change: func(settings *twitchrewards.Settings) { settings.Cost = 0 }},
		"a cost over the maximum":         {change: func(settings *twitchrewards.Settings) { settings.Cost = twitchrewards.MaximumCost + 1 }},
		"a colour that isn't hex":         {change: func(settings *twitchrewards.Settings) { settings.BackgroundColor = "purple" }},
		"a colour Twitch picks":           {change: func(settings *twitchrewards.Settings) { settings.BackgroundColor = "" }, isValid: true},
		"a negative cooldown":             {change: func(settings *twitchrewards.Settings) { settings.CooldownSeconds = -1 }},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			settings := hydrateSettings()
			settings.BackgroundColor = "#9147FF"
			testCase.change(&settings)

			err := settings.Validate()

			if testCase.isValid && err != nil {
				t.Errorf("got error %v, want valid", err)
			}
			if !testCase.isValid && !errors.Is(err, twitchrewards.ErrInvalidSettings) {
				t.Errorf("got error %v, want ErrInvalidSettings", err)
			}
		})
	}
}

func TestNewSettings_StartShownAndNotRefundable(t *testing.T) {
	settings := twitchrewards.NewSettings()

	if !settings.IsShownToViewers || settings.CanBeRefunded {
		t.Errorf("got %+v, want shown to viewers and not refundable", settings)
	}
}

func TestTwitchSourceCreate_SendsEveryLimitAndMarksTheRewardAsWhendos(t *testing.T) {
	source, api := newRecordingSource(t, http.StatusOK, changedRewardJSON)

	reward, err := source.Create(context.Background(), hydrateSettings())

	if err != nil || reward.ID != "new-reward" || !reward.IsMadeByWhendo {
		t.Fatalf("got reward %+v, error %v", reward, err)
	}
	request := api.received()[0]
	if request.method != http.MethodPost || request.query.Get("broadcaster_id") != streamerUserID || request.query.Has("id") {
		t.Errorf("got %s with query %v", request.method, request.query)
	}
	want := map[string]any{
		"title": "Hydrate", "prompt": "Drink water", "cost": 100.0, "is_enabled": true, "is_user_input_required": false,
		"should_redemptions_skip_request_queue": true,
		"is_max_per_stream_enabled":             false,
		"is_max_per_user_per_stream_enabled":    true, "max_per_user_per_stream": 2.0,
		"is_global_cooldown_enabled": true, "global_cooldown_seconds": 60.0,
	}
	if len(request.body) != len(want) {
		t.Errorf("got body %v, want %v", request.body, want)
	}
	for field, wantValue := range want {
		if request.body[field] != wantValue {
			t.Errorf("got %s = %v, want %v", field, request.body[field], wantValue)
		}
	}
}

func TestTwitchSourceCreate_RefusesInvalidSettingsWithoutCallingTwitch(t *testing.T) {
	source, api := newRecordingSource(t, http.StatusOK, changedRewardJSON)
	settings := hydrateSettings()
	settings.Title = ""

	_, err := source.Create(context.Background(), settings)

	if !errors.Is(err, twitchrewards.ErrInvalidSettings) || len(api.received()) != 0 {
		t.Errorf("got error %v after %d requests, want ErrInvalidSettings and none", err, len(api.received()))
	}
}

func TestTwitchSourceCreate_ReturnsNoChannelPointsForForbidden(t *testing.T) {
	source, _ := newRecordingSource(t, http.StatusForbidden, notAffiliateBody)

	_, err := source.Create(context.Background(), hydrateSettings())

	if !errors.Is(err, twitchrewards.ErrNoChannelPoints) {
		t.Errorf("got error %v, want ErrNoChannelPoints", err)
	}
}

func TestTwitchSourceCreate_PassesOnTwitchsReasonForRefusing(t *testing.T) {
	source, _ := newRecordingSource(t, http.StatusBadRequest, `{"message":"The title must be unique (case-insensitive) amongst all of the broadcaster's custom rewards."}`)

	_, err := source.Create(context.Background(), hydrateSettings())

	if err == nil || !strings.Contains(err.Error(), "must be unique") {
		t.Errorf("got error %v, want Twitch's reason", err)
	}
}

func TestTwitchSourceUpdate_PatchesTheRewardAndLeavesOutABlankPrompt(t *testing.T) {
	source, api := newRecordingSource(t, http.StatusOK, changedRewardJSON)
	settings := hydrateSettings()
	settings.Prompt = "   "

	_, err := source.Update(context.Background(), "new-reward", settings)

	if err != nil {
		t.Fatalf("got error %v", err)
	}
	request := api.received()[0]
	if request.method != http.MethodPatch || request.query.Get("id") != "new-reward" {
		t.Errorf("got %s with query %v", request.method, request.query)
	}
	if _, hasPrompt := request.body["prompt"]; hasPrompt {
		t.Errorf("got prompt %v, want it left out so Twitch keeps the old one", request.body["prompt"])
	}
	if request.body["is_max_per_stream_enabled"] != false || request.body["is_global_cooldown_enabled"] != true {
		t.Errorf("got body %v, want every limit sent", request.body)
	}
}

func TestTwitchSourceUpdate_ExplainsForbiddenAndNotFound(t *testing.T) {
	forbiddenSource, _ := newRecordingSource(t, http.StatusForbidden, `{"message":"Forbidden"}`)
	missingSource, _ := newRecordingSource(t, http.StatusNotFound, `{"message":"Not Found"}`)

	_, forbiddenErr := forbiddenSource.Update(context.Background(), "dashboard-reward", hydrateSettings())
	_, missingErr := missingSource.Update(context.Background(), "deleted-reward", hydrateSettings())

	if !errors.Is(forbiddenErr, twitchrewards.ErrNotMadeByWhendo) {
		t.Errorf("got error %v for 403, want ErrNotMadeByWhendo", forbiddenErr)
	}
	if !errors.Is(missingErr, twitchrewards.ErrRewardNotFound) {
		t.Errorf("got error %v for 404, want ErrRewardNotFound", missingErr)
	}
}

func TestTwitchSourceSetPaused_SendsOnlyThePause(t *testing.T) {
	source, api := newRecordingSource(t, http.StatusOK, changedRewardJSON)

	_, err := source.SetPaused(context.Background(), "new-reward", true)

	request := api.received()[0]
	if err != nil || request.method != http.MethodPatch || len(request.body) != 1 || request.body["is_paused"] != true {
		t.Errorf("got %s with body %v, error %v", request.method, request.body, err)
	}
}

func TestTwitchSourceDelete_DeletesTheReward(t *testing.T) {
	source, api := newRecordingSource(t, http.StatusNoContent, "")

	err := source.Delete(context.Background(), "new-reward")

	request := api.received()[0]
	if err != nil || request.method != http.MethodDelete || request.query.Get("id") != "new-reward" || request.body != nil {
		t.Errorf("got %s with query %v and body %v, error %v", request.method, request.query, request.body, err)
	}
}

func TestTwitchSourceDelete_ReturnsNotMadeByWhendoForForbidden(t *testing.T) {
	source, _ := newRecordingSource(t, http.StatusForbidden, `{"message":"Forbidden"}`)

	err := source.Delete(context.Background(), "dashboard-reward")

	if !errors.Is(err, twitchrewards.ErrNotMadeByWhendo) {
		t.Errorf("got error %v, want ErrNotMadeByWhendo", err)
	}
}

func findFakeReward(t *testing.T, source *twitchrewards.FakeSource, title string) (twitchrewards.Reward, bool) {
	t.Helper()
	rewards, err := source.List(context.Background())
	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}
	for _, reward := range rewards {
		if reward.Title == title {
			return reward, true
		}
	}
	return twitchrewards.Reward{}, false
}

func TestFakeSource_CreatesAWhendoRewardItCanThenChange(t *testing.T) {
	source := twitchrewards.NewFakeSource()
	settings := hydrateSettings()
	settings.Title = "Stretch"

	created, err := source.Create(context.Background(), settings)
	if err != nil {
		t.Fatalf("Create returned an error: %v", err)
	}
	settings.Cost = 250
	updated, updateErr := source.Update(context.Background(), created.ID, settings)
	paused, pauseErr := source.SetPaused(context.Background(), created.ID, true)

	if !created.IsMadeByWhendo || updateErr != nil || updated.Cost != 250 || pauseErr != nil || !paused.IsPaused {
		t.Errorf("got created %+v, updated %+v (%v), paused %+v (%v)", created, updated, updateErr, paused, pauseErr)
	}
	listed, isListed := findFakeReward(t, source, "Stretch")
	if !isListed || listed.Cost != 250 || !listed.IsPaused {
		t.Errorf("got listed %+v, want the changes kept", listed)
	}
}

func TestFakeSource_RefusesANameAlreadyInUseIgnoringCapitalization(t *testing.T) {
	source := twitchrewards.NewFakeSource()
	settings := hydrateSettings()
	settings.Title = "check in"

	_, err := source.Create(context.Background(), settings)

	if !errors.Is(err, twitchrewards.ErrTitleTaken) {
		t.Errorf("got error %v, want ErrTitleTaken", err)
	}
}

func TestFakeSource_LetsARewardKeepItsOwnName(t *testing.T) {
	source := twitchrewards.NewFakeSource()
	checkIn, isFound := findFakeReward(t, source, "Check In")
	if !isFound {
		t.Fatal("no Check In reward")
	}
	settings := checkIn.Settings()
	settings.Cost = 75

	_, err := source.Update(context.Background(), checkIn.ID, settings)

	if err != nil {
		t.Errorf("got error %v renaming a reward to its own name", err)
	}
}

func TestFakeSource_RefusesToChangeRewardsWhendoDidntMake(t *testing.T) {
	source := twitchrewards.NewFakeSource()
	dashboardReward, isFound := findFakeReward(t, source, "D20 Mute Me")
	if !isFound {
		t.Fatal("no D20 Mute Me reward")
	}

	_, updateErr := source.Update(context.Background(), dashboardReward.ID, dashboardReward.Settings())
	deleteErr := source.Delete(context.Background(), dashboardReward.ID)

	if !errors.Is(updateErr, twitchrewards.ErrNotMadeByWhendo) || !errors.Is(deleteErr, twitchrewards.ErrNotMadeByWhendo) {
		t.Errorf("got errors %v and %v, want ErrNotMadeByWhendo", updateErr, deleteErr)
	}
}

func TestFakeSource_DeletesAWhendoReward(t *testing.T) {
	source := twitchrewards.NewFakeSource()
	hydrate, isFound := findFakeReward(t, source, "Hydrate")
	if !isFound {
		t.Fatal("no Hydrate reward")
	}

	err := source.Delete(context.Background(), hydrate.ID)

	if _, isStillListed := findFakeReward(t, source, "Hydrate"); err != nil || isStillListed {
		t.Errorf("got error %v, still listed %v", err, isStillListed)
	}
	if err := source.Delete(context.Background(), hydrate.ID); !errors.Is(err, twitchrewards.ErrRewardNotFound) {
		t.Errorf("deleting again got error %v, want ErrRewardNotFound", err)
	}
}

func TestFakeSource_RefusesMoreRewardsThanTwitchAllows(t *testing.T) {
	source := twitchrewards.NewFakeSource()
	existing, err := source.List(context.Background())
	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}
	settings := hydrateSettings()
	for count := len(existing); count < twitchrewards.MaximumRewardsPerChannel; count++ {
		settings.Title = fmt.Sprintf("Filler %d", count)
		_, err = source.Create(context.Background(), settings)
		if err != nil {
			t.Fatalf("creating reward %d: %v", count, err)
		}
	}
	settings.Title = "One too many"

	_, err = source.Create(context.Background(), settings)

	if !errors.Is(err, twitchrewards.ErrTooManyRewards) {
		t.Errorf("got error %v, want ErrTooManyRewards", err)
	}
}
