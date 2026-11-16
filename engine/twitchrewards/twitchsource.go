package twitchrewards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/SloanGwaltney/whendo/engine/twitchapi"
)

const customRewardsPath = "/channel_points/custom_rewards"

// TwitchSource reads rewards from Twitch's API as the logged-in streamer.
type TwitchSource struct {
	twitch *twitchapi.Client
}

// NewTwitchSource returns a source that acts through the given client.
func NewTwitchSource(twitch *twitchapi.Client) *TwitchSource {
	return &TwitchSource{twitch: twitch}
}

// customRewardsResponse is the shape of Get Custom Reward's response.
type customRewardsResponse struct {
	Data []customReward `json:"data"`
}

type customReward struct {
	ID                                string `json:"id"`
	Title                             string `json:"title"`
	Prompt                            string `json:"prompt"`
	Cost                              int64  `json:"cost"`
	BackgroundColor                   string `json:"background_color"`
	IsEnabled                         bool   `json:"is_enabled"`
	IsPaused                          bool   `json:"is_paused"`
	IsUserInputRequired               bool   `json:"is_user_input_required"`
	ShouldRedemptionsSkipRequestQueue bool   `json:"should_redemptions_skip_request_queue"`
	MaxPerStreamSetting               struct {
		IsEnabled    bool  `json:"is_enabled"`
		MaxPerStream int64 `json:"max_per_stream"`
	} `json:"max_per_stream_setting"`
	MaxPerUserPerStreamSetting struct {
		IsEnabled           bool  `json:"is_enabled"`
		MaxPerUserPerStream int64 `json:"max_per_user_per_stream"`
	} `json:"max_per_user_per_stream_setting"`
	GlobalCooldownSetting struct {
		IsEnabled             bool  `json:"is_enabled"`
		GlobalCooldownSeconds int64 `json:"global_cooldown_seconds"`
	} `json:"global_cooldown_setting"`
}

// List asks Twitch twice: once for every reward, and once for the ones
// Whendo can manage, which are exactly the ones Whendo created. Twitch is
// the record of which redeems Whendo made, so nothing here can drift from
// it.
func (source *TwitchSource) List(ctx context.Context) ([]Reward, error) {
	everyReward, err := source.fetch(ctx, false)
	if err != nil {
		return nil, err
	}
	manageableRewards, err := source.fetch(ctx, true)
	if err != nil {
		return nil, err
	}
	manageableIDs := make(map[string]bool, len(manageableRewards))
	for _, manageable := range manageableRewards {
		manageableIDs[manageable.ID] = true
	}
	rewards := make([]Reward, 0, len(everyReward))
	for _, twitchReward := range everyReward {
		rewards = append(rewards, twitchReward.toReward(manageableIDs[twitchReward.ID]))
	}
	sortForShowing(rewards)
	return rewards, nil
}

func (source *TwitchSource) fetch(ctx context.Context, isOnlyManageable bool) ([]customReward, error) {
	broadcasterUserID, err := source.twitch.BroadcasterUserID()
	if err != nil {
		return nil, err
	}
	query := url.Values{"broadcaster_id": {broadcasterUserID}}
	if isOnlyManageable {
		query.Set("only_manageable_rewards", "true")
	}
	response, err := source.twitch.Do(ctx, twitchapi.Request{Method: http.MethodGet, Path: customRewardsPath, Query: query})
	if err != nil {
		return nil, err
	}
	// Twitch documents no other reason for a 403 here, so it's taken to mean
	// the account has no channel points rather than checked separately. See
	// docs/design/twitch-redeems.md#accounts-without-channel-points.
	if response.StatusCode == http.StatusForbidden {
		return nil, ErrNoChannelPoints
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch couldn't list your redeems (HTTP %d): %s", response.StatusCode, response.ErrorMessage())
	}
	return decodeRewards(response.Body)
}

func decodeRewards(body []byte) ([]customReward, error) {
	var decodedResponse customRewardsResponse
	err := json.Unmarshal(body, &decodedResponse)
	if err != nil {
		return nil, fmt.Errorf("reading Twitch's answer about redeems: %w", err)
	}
	return decodedResponse.Data, nil
}

// toReward keeps a limit's value only while the limit is on, since Twitch
// remembers the value of a limit that's been turned off.
func (twitchReward customReward) toReward(isMadeByWhendo bool) Reward {
	reward := Reward{
		ID:                  twitchReward.ID,
		Title:               twitchReward.Title,
		Prompt:              twitchReward.Prompt,
		Cost:                twitchReward.Cost,
		BackgroundColor:     twitchReward.BackgroundColor,
		IsShownToViewers:    twitchReward.IsEnabled,
		IsPaused:            twitchReward.IsPaused,
		IsUserInputRequired: twitchReward.IsUserInputRequired,
		CanBeRefunded:       !twitchReward.ShouldRedemptionsSkipRequestQueue,
		IsMadeByWhendo:      isMadeByWhendo,
	}
	if twitchReward.MaxPerStreamSetting.IsEnabled {
		reward.MaximumPerStream = twitchReward.MaxPerStreamSetting.MaxPerStream
	}
	if twitchReward.MaxPerUserPerStreamSetting.IsEnabled {
		reward.MaximumPerViewerPerStream = twitchReward.MaxPerUserPerStreamSetting.MaxPerUserPerStream
	}
	if twitchReward.GlobalCooldownSetting.IsEnabled {
		reward.CooldownSeconds = twitchReward.GlobalCooldownSetting.GlobalCooldownSeconds
	}
	return reward
}

// rewardRequestBody is what Create Custom Rewards and Update Custom Reward
// take. Every limit is sent each time: Twitch turns off a limit it isn't
// sent, so leaving one out would quietly turn it off. A limit that's off
// sends only its switch, since a value of zero isn't valid.
type rewardRequestBody struct {
	Title                             string `json:"title"`
	Prompt                            string `json:"prompt,omitempty"`
	Cost                              int64  `json:"cost"`
	BackgroundColor                   string `json:"background_color,omitempty"`
	IsEnabled                         bool   `json:"is_enabled"`
	IsUserInputRequired               bool   `json:"is_user_input_required"`
	ShouldRedemptionsSkipRequestQueue bool   `json:"should_redemptions_skip_request_queue"`
	IsMaxPerStreamEnabled             bool   `json:"is_max_per_stream_enabled"`
	MaxPerStream                      int64  `json:"max_per_stream,omitempty"`
	IsMaxPerUserPerStreamEnabled      bool   `json:"is_max_per_user_per_stream_enabled"`
	MaxPerUserPerStream               int64  `json:"max_per_user_per_stream,omitempty"`
	IsGlobalCooldownEnabled           bool   `json:"is_global_cooldown_enabled"`
	GlobalCooldownSeconds             int64  `json:"global_cooldown_seconds,omitempty"`
}

// newRewardRequestBody leaves out a blank prompt, since Twitch keeps the old
// prompt when sent an empty one.
func newRewardRequestBody(settings Settings) rewardRequestBody {
	prompt := settings.Prompt
	if strings.TrimSpace(prompt) == "" {
		prompt = ""
	}
	return rewardRequestBody{
		Title:                             strings.TrimSpace(settings.Title),
		Prompt:                            prompt,
		Cost:                              settings.Cost,
		BackgroundColor:                   settings.BackgroundColor,
		IsEnabled:                         settings.IsShownToViewers,
		IsUserInputRequired:               settings.IsUserInputRequired,
		ShouldRedemptionsSkipRequestQueue: !settings.CanBeRefunded,
		IsMaxPerStreamEnabled:             settings.MaximumPerStream > 0,
		MaxPerStream:                      settings.MaximumPerStream,
		IsMaxPerUserPerStreamEnabled:      settings.MaximumPerViewerPerStream > 0,
		MaxPerUserPerStream:               settings.MaximumPerViewerPerStream,
		IsGlobalCooldownEnabled:           settings.CooldownSeconds > 0,
		GlobalCooldownSeconds:             settings.CooldownSeconds,
	}
}

type pauseRequestBody struct {
	IsPaused bool `json:"is_paused"`
}

// Create asks Twitch to make the reward. Twitch only lets the app that made
// a reward change it, so a reward made here is one Whendo can edit.
func (source *TwitchSource) Create(ctx context.Context, settings Settings) (Reward, error) {
	err := settings.Validate()
	if err != nil {
		return Reward{}, err
	}
	return source.change(ctx, http.MethodPost, "", newRewardRequestBody(settings))
}

// Update replaces a reward's settings.
func (source *TwitchSource) Update(ctx context.Context, rewardID string, settings Settings) (Reward, error) {
	err := settings.Validate()
	if err != nil {
		return Reward{}, err
	}
	return source.change(ctx, http.MethodPatch, rewardID, newRewardRequestBody(settings))
}

// SetPaused pauses or resumes a reward.
func (source *TwitchSource) SetPaused(ctx context.Context, rewardID string, isPaused bool) (Reward, error) {
	return source.change(ctx, http.MethodPatch, rewardID, pauseRequestBody{IsPaused: isPaused})
}

// Delete removes a reward.
func (source *TwitchSource) Delete(ctx context.Context, rewardID string) error {
	response, err := source.send(ctx, http.MethodDelete, rewardID, nil)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusNoContent {
		return changeError(response, rewardID)
	}
	return nil
}

// change creates a reward, or changes the one with rewardID, and returns it
// as Twitch now has it.
func (source *TwitchSource) change(ctx context.Context, method string, rewardID string, body any) (Reward, error) {
	response, err := source.send(ctx, method, rewardID, body)
	if err != nil {
		return Reward{}, err
	}
	if response.StatusCode != http.StatusOK {
		return Reward{}, changeError(response, rewardID)
	}
	rewards, err := decodeRewards(response.Body)
	if err != nil {
		return Reward{}, err
	}
	if len(rewards) == 0 {
		return Reward{}, errors.New("twitch's answer didn't include the redeem")
	}
	return rewards[0].toReward(true), nil
}

// send sends a request about one reward, or about the channel's rewards
// when rewardID is empty.
func (source *TwitchSource) send(ctx context.Context, method string, rewardID string, body any) (twitchapi.Response, error) {
	broadcasterUserID, err := source.twitch.BroadcasterUserID()
	if err != nil {
		return twitchapi.Response{}, err
	}
	query := url.Values{"broadcaster_id": {broadcasterUserID}}
	if rewardID != "" {
		query.Set("id", rewardID)
	}
	return source.twitch.Do(ctx, twitchapi.Request{Method: method, Path: customRewardsPath, Query: query, Body: body})
}

// changeError explains why Twitch refused to create or change a reward;
// rewardID is empty when creating. A 400 carries Twitch's own reason, such
// as a name already in use or one AutoMod rejected, so it's passed on as
// Twitch wrote it.
func changeError(response twitchapi.Response, rewardID string) error {
	switch response.StatusCode {
	case http.StatusBadRequest:
		return fmt.Errorf("twitch refused the redeem: %s", response.ErrorMessage())
	// Twitch answers 403 both for a reward another app made and for an
	// account without channel points. Creating has no reward yet, so it can
	// only be the second. Changing a reward that was listed can only be the
	// first.
	case http.StatusForbidden:
		if rewardID == "" {
			return ErrNoChannelPoints
		}
		return ErrNotMadeByWhendo
	case http.StatusNotFound:
		return ErrRewardNotFound
	default:
		return fmt.Errorf("twitch couldn't change the redeem (HTTP %d): %s", response.StatusCode, response.ErrorMessage())
	}
}
