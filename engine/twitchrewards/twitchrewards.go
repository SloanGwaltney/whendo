// Package twitchrewards reads, and later creates and edits, the streamer's
// channel point rewards, which Whendo's UI calls redeems. A Source stands
// for where rewards come from: Twitch itself, or a fake for developing on an
// account without channel points. See docs/design/twitch-redeems.md.
package twitchrewards

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
)

// ErrNoChannelPoints means the account can't have channel point rewards,
// because it isn't a Twitch affiliate or partner.
var ErrNoChannelPoints = errors.New("this Twitch account doesn't have channel points, which need affiliate or partner status")

// Reward is one channel point reward, as the Redeems page and the redeem
// picker show it. Limits of zero are turned off.
type Reward struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Prompt          string `json:"prompt"`
	Cost            int64  `json:"cost"`
	BackgroundColor string `json:"backgroundColor"`
	// IsShownToViewers is Twitch's is_enabled: whether viewers see it at all.
	IsShownToViewers bool `json:"isShownToViewers"`
	// IsPaused means viewers see it, but can't redeem it.
	IsPaused            bool `json:"isPaused"`
	IsUserInputRequired bool `json:"isUserInputRequired"`
	// CanBeRefunded is the opposite of Twitch's
	// should_redemptions_skip_request_queue. See
	// docs/design/twitch-redeems.md#refunds-and-the-request-queue.
	CanBeRefunded             bool  `json:"canBeRefunded"`
	MaximumPerStream          int64 `json:"maximumPerStream"`
	MaximumPerViewerPerStream int64 `json:"maximumPerViewerPerStream"`
	CooldownSeconds           int64 `json:"cooldownSeconds"`
	// IsMadeByWhendo means Whendo created it, so Whendo can edit it, and
	// refund and fulfill its redemptions. Twitch only allows that for the
	// app that created a reward.
	IsMadeByWhendo bool `json:"isMadeByWhendo"`
}

// Source is where rewards come from.
type Source interface {
	// List returns every reward the account has, cheapest first. It returns
	// ErrNoChannelPoints for an account that can't have rewards.
	List(ctx context.Context) ([]Reward, error)
}

// sortForShowing orders rewards the way Twitch shows them to viewers:
// cheapest first, then by title.
func sortForShowing(rewards []Reward) {
	slices.SortFunc(rewards, func(first Reward, second Reward) int {
		return cmp.Or(
			cmp.Compare(first.Cost, second.Cost),
			cmp.Compare(strings.ToLower(first.Title), strings.ToLower(second.Title)),
		)
	})
}
