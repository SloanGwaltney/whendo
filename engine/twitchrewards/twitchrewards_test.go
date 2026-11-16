package twitchrewards_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/twitchapi"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
)

const (
	testClientID   = "test-client-id"
	streamerUserID = "1337"
	accessToken    = "token"

	// Two rewards in Twitch's shape: an expensive one Whendo made, with every
	// limit on, and a cheap one made in the dashboard, with its limits off
	// but values Twitch still remembers.
	whendoRewardJSON = `{
		"id": "made-by-whendo", "title": "Do a Voice", "prompt": "Pick a voice", "cost": 1000,
		"background_color": "#9147FF", "is_enabled": true, "is_paused": true, "is_user_input_required": true,
		"should_redemptions_skip_request_queue": false,
		"max_per_stream_setting": {"is_enabled": true, "max_per_stream": 5},
		"max_per_user_per_stream_setting": {"is_enabled": true, "max_per_user_per_stream": 1},
		"global_cooldown_setting": {"is_enabled": true, "global_cooldown_seconds": 300}
	}`
	dashboardRewardJSON = `{
		"id": "made-in-dashboard", "title": "Hydrate", "prompt": "", "cost": 100,
		"background_color": "#00C7AC", "is_enabled": false, "is_paused": false, "is_user_input_required": false,
		"should_redemptions_skip_request_queue": true,
		"max_per_stream_setting": {"is_enabled": false, "max_per_stream": 9},
		"max_per_user_per_stream_setting": {"is_enabled": false, "max_per_user_per_stream": 9},
		"global_cooldown_setting": {"is_enabled": false, "global_cooldown_seconds": 9}
	}`
	notAffiliateBody = `{"error":"Forbidden","status":403,"message":"The broadcaster is not a partner or affiliate."}`
)

type fakeAccount struct {
	isLoggedIn bool
}

func (account fakeAccount) AccountUserID() (string, error) {
	if !account.isLoggedIn {
		return "", twitchauth.ErrNotLoggedIn
	}
	return streamerUserID, nil
}

func (account fakeAccount) AccessToken(context.Context) (string, error) {
	if !account.isLoggedIn {
		return "", twitchauth.ErrNotLoggedIn
	}
	return accessToken, nil
}

func (account fakeAccount) RenewAccessToken(context.Context, string) (string, error) {
	return accessToken, nil
}

// fakeRewardsAPI answers Get Custom Reward. The two lists are what it
// returns without and with only_manageable_rewards.
type fakeRewardsAPI struct {
	t               *testing.T
	statusCode      int
	everyRewardJSON string
	manageableJSON  string
	unexpectedBody  string
}

func (fake *fakeRewardsAPI) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || request.URL.Path != "/channel_points/custom_rewards" {
		fake.t.Errorf("got %s %s, want GET /channel_points/custom_rewards", request.Method, request.URL.Path)
	}
	if request.URL.Query().Get("broadcaster_id") != streamerUserID {
		fake.t.Errorf("got broadcaster_id %q", request.URL.Query().Get("broadcaster_id"))
	}
	writer.WriteHeader(fake.statusCode)
	body := "[" + fake.everyRewardJSON + "]"
	if request.URL.Query().Get("only_manageable_rewards") == "true" {
		body = "[" + fake.manageableJSON + "]"
	}
	if fake.statusCode != http.StatusOK {
		body = fake.unexpectedBody
	} else {
		body = `{"data":` + body + `}`
	}
	_, err := writer.Write([]byte(body))
	if err != nil {
		fake.t.Errorf("writing the response: %v", err)
	}
}

func newTwitchSource(t *testing.T, fake *fakeRewardsAPI, account fakeAccount) *twitchrewards.TwitchSource {
	t.Helper()
	fake.t = t
	server := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(server.Close)
	client := twitchapi.NewClient(twitchapi.Config{ClientID: testClientID, Account: account, BaseURL: server.URL, HTTPClient: server.Client()})
	return twitchrewards.NewTwitchSource(client)
}

func TestTwitchSource_ListMarksWhendoRewardsAndSortsCheapestFirst(t *testing.T) {
	fake := &fakeRewardsAPI{statusCode: http.StatusOK, everyRewardJSON: whendoRewardJSON + "," + dashboardRewardJSON, manageableJSON: whendoRewardJSON}
	source := newTwitchSource(t, fake, fakeAccount{isLoggedIn: true})

	rewards, err := source.List(context.Background())

	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}
	if len(rewards) != 2 || rewards[0].ID != "made-in-dashboard" || rewards[1].ID != "made-by-whendo" {
		t.Fatalf("got rewards %+v, want the dashboard reward first since it's cheaper", rewards)
	}
	if rewards[0].IsMadeByWhendo || !rewards[1].IsMadeByWhendo {
		t.Errorf("got made by Whendo %v and %v, want only the manageable reward marked", rewards[0].IsMadeByWhendo, rewards[1].IsMadeByWhendo)
	}
}

func TestTwitchSource_ListReadsEverySetting(t *testing.T) {
	fake := &fakeRewardsAPI{statusCode: http.StatusOK, everyRewardJSON: whendoRewardJSON, manageableJSON: whendoRewardJSON}
	source := newTwitchSource(t, fake, fakeAccount{isLoggedIn: true})

	rewards, err := source.List(context.Background())

	want := twitchrewards.Reward{
		ID: "made-by-whendo", Title: "Do a Voice", Prompt: "Pick a voice", Cost: 1000, BackgroundColor: "#9147FF",
		IsShownToViewers: true, IsPaused: true, IsUserInputRequired: true, CanBeRefunded: true,
		MaximumPerStream: 5, MaximumPerViewerPerStream: 1, CooldownSeconds: 300, IsMadeByWhendo: true,
	}
	if err != nil || len(rewards) != 1 || rewards[0] != want {
		t.Errorf("got %+v, error %v; want %+v", rewards, err, want)
	}
}

func TestTwitchSource_ListTreatsLimitsThatAreOffAsZero(t *testing.T) {
	fake := &fakeRewardsAPI{statusCode: http.StatusOK, everyRewardJSON: dashboardRewardJSON, manageableJSON: ""}
	source := newTwitchSource(t, fake, fakeAccount{isLoggedIn: true})

	rewards, err := source.List(context.Background())

	if err != nil || len(rewards) != 1 {
		t.Fatalf("got %+v, error %v", rewards, err)
	}
	reward := rewards[0]
	if reward.MaximumPerStream != 0 || reward.MaximumPerViewerPerStream != 0 || reward.CooldownSeconds != 0 || reward.CanBeRefunded || reward.IsShownToViewers {
		t.Errorf("got %+v, want every limit off, not refundable and hidden", reward)
	}
}

func TestTwitchSource_ListReturnsNoChannelPointsForForbidden(t *testing.T) {
	fake := &fakeRewardsAPI{statusCode: http.StatusForbidden, unexpectedBody: notAffiliateBody}
	source := newTwitchSource(t, fake, fakeAccount{isLoggedIn: true})

	_, err := source.List(context.Background())

	if !errors.Is(err, twitchrewards.ErrNoChannelPoints) {
		t.Errorf("got error %v, want ErrNoChannelPoints", err)
	}
}

func TestTwitchSource_ListReturnsTwitchsMessageForOtherErrors(t *testing.T) {
	fake := &fakeRewardsAPI{statusCode: http.StatusInternalServerError, unexpectedBody: `{"message":"something broke"}`}
	source := newTwitchSource(t, fake, fakeAccount{isLoggedIn: true})

	_, err := source.List(context.Background())

	if err == nil || errors.Is(err, twitchrewards.ErrNoChannelPoints) || !strings.Contains(err.Error(), "something broke") {
		t.Errorf("got error %v, want Twitch's message", err)
	}
}

func TestTwitchSource_ListReturnsNotLoggedIn(t *testing.T) {
	source := newTwitchSource(t, &fakeRewardsAPI{statusCode: http.StatusOK}, fakeAccount{isLoggedIn: false})

	_, err := source.List(context.Background())

	if !errors.Is(err, twitchapi.ErrNotLoggedIn) {
		t.Errorf("got error %v, want ErrNotLoggedIn", err)
	}
}

func TestFakeSource_ListHasRewardsOfBothKinds(t *testing.T) {
	rewards, err := twitchrewards.NewFakeSource().List(context.Background())

	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}
	var whendoCount, dashboardCount int
	for _, reward := range rewards {
		if reward.IsMadeByWhendo {
			whendoCount++
		} else {
			dashboardCount++
		}
	}
	if whendoCount == 0 || dashboardCount == 0 {
		t.Errorf("got %d Whendo rewards and %d others, want some of each", whendoCount, dashboardCount)
	}
}

func TestFakeSource_ListReturnsACopy(t *testing.T) {
	source := twitchrewards.NewFakeSource()
	rewards, err := source.List(context.Background())
	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}

	rewards[0].Title = "changed by the caller"
	listedAgain, err := source.List(context.Background())

	if err != nil || listedAgain[0].Title == "changed by the caller" {
		t.Errorf("a caller's change reached the source: %+v, error %v", listedAgain, err)
	}
}
