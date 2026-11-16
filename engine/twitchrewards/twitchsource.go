package twitchrewards

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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
	var decodedResponse customRewardsResponse
	err = json.Unmarshal(response.Body, &decodedResponse)
	if err != nil {
		return nil, fmt.Errorf("reading Twitch's list of redeems: %w", err)
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
