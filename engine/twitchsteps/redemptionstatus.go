package twitchsteps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/twitchapi"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
)

const (
	// RefundRedemptionStepID is the step that refunds the redemption that
	// started the automation, giving the viewer their channel points back.
	RefundRedemptionStepID = "twitch.refundRedemption"
	// FulfillRedemptionStepID is the step that marks the redemption that
	// started the automation as fulfilled, so it can't be refunded.
	FulfillRedemptionStepID = "twitch.fulfillRedemption"

	redemptionsPath = "/channel_points/custom_rewards/redemptions"
	// Twitch's statuses for a redemption that's refunded or fulfilled.
	canceledStatus  = "CANCELED"
	fulfilledStatus = "FULFILLED"

	// redeemsGroup is the step picker's group for steps that act on redeems.
	redeemsGroup = "Redeems"
)

// RewardLookup tells what the last list of rewards said about a reward, so
// the refund and fulfill steps can point out a redeem whose redemptions
// Twitch won't let Whendo change. It reports false for a reward it doesn't
// know, including when no list has loaded.
type RewardLookup interface {
	KnownReward(rewardID string) (twitchrewards.Reward, bool)
}

// redemptionStatusStep sets the status of the redemption that started the
// automation. Refunding and fulfilling differ only in the status they set
// and how they're worded. See docs/design/twitch-steps.md#refund-and-fulfill.
type redemptionStatusStep struct {
	twitch  *twitchapi.Client
	rewards RewardLookup
	wording redemptionStatusWording
}

// redemptionStatusWording is what differs between refunding and fulfilling.
type redemptionStatusWording struct {
	stepID      string
	displayName string
	description string
	// status is the status Twitch gives the redemption.
	status string
	// verb and pastParticiple word failures, such as "refund" and
	// "refunded".
	verb           string
	pastParticiple string
	// cantBeRefundedNote is the note for a redeem that skips Twitch's
	// request queue, whose redemptions are fulfilled the moment they're made.
	cantBeRefundedNote string
}

var refundWording = redemptionStatusWording{
	stepID:             RefundRedemptionStepID,
	displayName:        "Refund the redemption",
	description:        "Give the viewer back the channel points they spent on the redeem that started this automation.",
	status:             canceledStatus,
	verb:               "refund",
	pastParticiple:     "refunded",
	cantBeRefundedNote: "This redeem can't be refunded. Turn on “Viewers can be refunded” in Redeems.",
}

var fulfillWording = redemptionStatusWording{
	stepID:             FulfillRedemptionStepID,
	displayName:        "Fulfill the redemption",
	description:        "Mark the redeem that started this automation as done in Twitch, so it can no longer be refunded.",
	status:             fulfilledStatus,
	verb:               "fulfill",
	pastParticiple:     "fulfilled",
	cantBeRefundedNote: "This redeem is fulfilled as soon as it's redeemed, so there's nothing left to fulfill. Turn on “Viewers can be refunded” in Redeems if you want this step to do it instead.",
}

type updateRedemptionStatusRequest struct {
	Status string `json:"status"`
}

func (statusStep redemptionStatusStep) Definition() step.Definition {
	return step.Definition{
		ID:                statusStep.wording.stepID,
		DisplayName:       statusStep.wording.displayName,
		Description:       statusStep.wording.description,
		Group:             redeemsGroup,
		Sentence:          automationsetting.Sentence{automationsetting.Words(statusStep.wording.displayName)},
		RequiredTriggerID: twitcheventsub.RedeemTriggerID,
	}
}

func (redemptionStatusStep) Outcomes(automationsetting.Values) ([]step.Outcome, error) {
	return []step.Outcome{{ID: step.DefaultOutcomeID, DisplayName: "Done"}}, nil
}

// NoteStep points out a redeem whose redemptions Twitch won't let Whendo
// change, going by the last list of redeems. A redeem the list doesn't
// have, such as the placeholder, gets no note: the trigger already explains
// those.
func (statusStep redemptionStatusStep) NoteStep(usage step.Usage) []step.Note {
	rewardID, err := usage.TriggerSettings.Text(twitcheventsub.RewardSetting)
	if err != nil {
		return nil
	}
	reward, isKnown := statusStep.rewards.KnownReward(rewardID)
	if !isKnown {
		return nil
	}
	if !reward.IsMadeByWhendo {
		return []step.Note{{Message: fmt.Sprintf("Whendo didn't make this redeem, so Twitch won't let it %s redemptions. Recreate the redeem in Whendo on the Redeems page.", statusStep.wording.verb)}}
	}
	if !reward.CanBeRefunded {
		return []step.Note{{Message: statusStep.wording.cantBeRefundedNote}}
	}
	return nil
}

// Run sets the status of the redemption that started the automation. A test
// run has no real redemption, so it fails without asking Twitch, with a
// reason that says so rather than Twitch's.
func (statusStep redemptionStatusStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	redemptionID, rewardID, err := redemptionFromTrigger(input.Variables)
	if err != nil {
		return step.Result{}, err
	}
	if redemptionID == twitcheventsub.TestEventID {
		return step.Result{}, fmt.Errorf("a test run has no real redemption to %s, so this step only works when someone really redeems", statusStep.wording.verb)
	}
	broadcasterUserID, err := statusStep.twitch.BroadcasterUserID()
	if err != nil {
		return step.Result{}, err
	}
	response, err := statusStep.twitch.Do(ctx, twitchapi.Request{
		Method: http.MethodPatch,
		Path:   redemptionsPath,
		Query: url.Values{
			"broadcaster_id": {broadcasterUserID},
			"reward_id":      {rewardID},
			"id":             {redemptionID},
		},
		Body: updateRedemptionStatusRequest{Status: statusStep.wording.status},
	})
	if err != nil {
		return step.Result{}, err
	}
	err = statusStep.checkResponse(response)
	if err != nil {
		return step.Result{}, err
	}
	return step.Result{OutcomeID: step.DefaultOutcomeID}, nil
}

// redemptionFromTrigger reads the redemption the redeem trigger provides.
// The check only lets these steps follow that trigger, so a missing value
// means a bug rather than a mistake the streamer made.
func redemptionFromTrigger(variables automationsetting.VariableValues) (redemptionID string, rewardID string, err error) {
	redemptionID, err = variables.Value(triggerVariable(twitcheventsub.RedemptionIDVariable))
	if err != nil {
		return "", "", fmt.Errorf("reading the redemption that started the automation: %w", err)
	}
	rewardID, err = variables.Value(triggerVariable(twitcheventsub.RewardIDVariable))
	if err != nil {
		return "", "", fmt.Errorf("reading the redeem that started the automation: %w", err)
	}
	if redemptionID == "" || rewardID == "" {
		return "", "", errors.New("the automation wasn't started by a redemption")
	}
	return redemptionID, rewardID, nil
}

func triggerVariable(variableID string) automationsetting.VariableReference {
	return automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: variableID}
}

// checkResponse turns Twitch's answer into a failure the streamer can
// understand.
func (statusStep redemptionStatusStep) checkResponse(response twitchapi.Response) error {
	verb := statusStep.wording.verb
	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("twitch didn't accept Whendo's login for changing redemptions: %s", response.ErrorMessage())
	case http.StatusForbidden:
		return fmt.Errorf("twitch only lets Whendo %s redemptions of redeems Whendo made, so recreate this redeem in Whendo on the Redeems page: %s", verb, response.ErrorMessage())
	case http.StatusNotFound:
		return fmt.Errorf("twitch has no redemption waiting to be %s: it may have been fulfilled or refunded already, or the redeem may not let viewers be refunded", statusStep.wording.pastParticiple)
	default:
		return fmt.Errorf("twitch couldn't %s the redemption (HTTP %d): %s", verb, response.StatusCode, response.ErrorMessage())
	}
}
