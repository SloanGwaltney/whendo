package twitchrewards

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Twitch's limits on a reward. See docs/design/twitch-redeems.md#creating.
const (
	MaximumTitleLength       = 45
	MinimumCost              = 1
	MaximumCost              = 1_000_000_000
	MaximumRewardsPerChannel = 50
)

var (
	// ErrInvalidSettings means a reward's settings break one of Twitch's
	// rules, which Whendo checks before asking Twitch.
	ErrInvalidSettings = errors.New("invalid redeem settings")
	// ErrRewardNotFound means the reward was deleted in Twitch.
	ErrRewardNotFound = errors.New("this redeem no longer exists in Twitch")
	// ErrNotMadeByWhendo means Twitch won't let Whendo change a reward,
	// because another app, such as the Twitch dashboard, made it.
	ErrNotMadeByWhendo = errors.New("twitch only lets the app that made a redeem change it, and Whendo didn't make this one")
	// ErrTooManyRewards means the channel already has as many rewards as
	// Twitch allows.
	ErrTooManyRewards = fmt.Errorf("twitch allows at most %d redeems per channel, shown and hidden together", MaximumRewardsPerChannel)
	// ErrTitleTaken means another reward already has the title.
	ErrTitleTaken = errors.New("another redeem already has this name")
)

// hexColorPattern is the colour format Twitch accepts, such as "#9147FF".
var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Settings are the parts of a reward the Redeems page edits. Limits of zero
// are turned off. Pausing is separate, since it's done from the redeem's row
// rather than the form.
type Settings struct {
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	Cost   int64  `json:"cost"`
	// BackgroundColor is empty to let Twitch pick one when creating.
	BackgroundColor           string `json:"backgroundColor"`
	IsShownToViewers          bool   `json:"isShownToViewers"`
	IsUserInputRequired       bool   `json:"isUserInputRequired"`
	CanBeRefunded             bool   `json:"canBeRefunded"`
	MaximumPerStream          int64  `json:"maximumPerStream"`
	MaximumPerViewerPerStream int64  `json:"maximumPerViewerPerStream"`
	CooldownSeconds           int64  `json:"cooldownSeconds"`
}

// NewSettings returns the settings a new redeem starts with. It's shown to
// viewers, and can't be refunded, since most redeems never need a refund and
// ones that can be pile up in the streamer's request queue. See
// docs/design/twitch-redeems.md#refunds-and-the-request-queue.
func NewSettings() Settings {
	return Settings{IsShownToViewers: true, CanBeRefunded: false}
}

// Validate checks the rules Whendo knows before asking Twitch, which checks
// them again, along with ones Whendo can't, such as AutoMod.
func (settings Settings) Validate() error {
	title := strings.TrimSpace(settings.Title)
	if title == "" {
		return fmt.Errorf("%w: give the redeem a name", ErrInvalidSettings)
	}
	if utf8.RuneCountInString(title) > MaximumTitleLength {
		return fmt.Errorf("%w: the name can be at most %d characters long", ErrInvalidSettings, MaximumTitleLength)
	}
	if settings.Cost < MinimumCost || settings.Cost > MaximumCost {
		return fmt.Errorf("%w: the cost has to be from %d to %d points", ErrInvalidSettings, MinimumCost, MaximumCost)
	}
	if settings.BackgroundColor != "" && !hexColorPattern.MatchString(settings.BackgroundColor) {
		return fmt.Errorf("%w: the colour has to look like #9147FF", ErrInvalidSettings)
	}
	if settings.MaximumPerStream < 0 || settings.MaximumPerViewerPerStream < 0 || settings.CooldownSeconds < 0 {
		return fmt.Errorf("%w: limits can't be negative", ErrInvalidSettings)
	}
	return nil
}

// Settings returns a reward's editable settings.
func (reward Reward) Settings() Settings {
	return Settings{
		Title:                     reward.Title,
		Prompt:                    reward.Prompt,
		Cost:                      reward.Cost,
		BackgroundColor:           reward.BackgroundColor,
		IsShownToViewers:          reward.IsShownToViewers,
		IsUserInputRequired:       reward.IsUserInputRequired,
		CanBeRefunded:             reward.CanBeRefunded,
		MaximumPerStream:          reward.MaximumPerStream,
		MaximumPerViewerPerStream: reward.MaximumPerViewerPerStream,
		CooldownSeconds:           reward.CooldownSeconds,
	}
}

// withSettings returns the reward with its editable settings replaced. A
// blank prompt keeps the old one, as Twitch does.
func (reward Reward) withSettings(settings Settings) Reward {
	prompt := reward.Prompt
	if strings.TrimSpace(settings.Prompt) != "" {
		prompt = settings.Prompt
	}
	reward.Title = strings.TrimSpace(settings.Title)
	reward.Prompt = prompt
	reward.Cost = settings.Cost
	if settings.BackgroundColor != "" {
		reward.BackgroundColor = settings.BackgroundColor
	}
	reward.IsShownToViewers = settings.IsShownToViewers
	reward.IsUserInputRequired = settings.IsUserInputRequired
	reward.CanBeRefunded = settings.CanBeRefunded
	reward.MaximumPerStream = settings.MaximumPerStream
	reward.MaximumPerViewerPerStream = settings.MaximumPerViewerPerStream
	reward.CooldownSeconds = settings.CooldownSeconds
	return reward
}
