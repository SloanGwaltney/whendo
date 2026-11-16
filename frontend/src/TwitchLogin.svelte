<script lang="ts">
  import {TwitchLoginService} from "../bindings/github.com/SloanGwaltney/whendo";
  import {LoginStatus, LogoutReason} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchauth";
  import type {LoginState} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchauth";

  let {loginState}: {loginState: LoginState} = $props();

  let isStarting = $state(false);
  let actionErrorMessage = $state("");

  const isWaitingForApproval = $derived(loginState.status === LoginStatus.LoginStatusWaitingForApproval);

  const logoutExplanation = $derived.by(() => {
    switch (loginState.logoutReason) {
      case LogoutReason.LogoutReasonCodeExpired:
        return "The login code expired before it was approved. Start again whenever you're ready.";
      case LogoutReason.LogoutReasonSessionEnded:
        return "Twitch logged you out. This happens when Whendo hasn't been opened for 30 days, or when it's disconnected in your Twitch settings. Log in again to pick up where you left off.";
      case LogoutReason.LogoutReasonLoginFailed:
        return "Something went wrong while logging in to Twitch. Please try again.";
      default:
        return "Whendo needs your Twitch account to react to redeems, chat commands and follows.";
    }
  });

  async function startLogin(): Promise<void> {
    actionErrorMessage = "";
    isStarting = true;
    try {
      await TwitchLoginService.StartLogin();
    } catch (error) {
      actionErrorMessage = `Couldn't reach Twitch. Check your internet connection and try again. (${error})`;
    } finally {
      isStarting = false;
    }
  }

  async function openTwitchAgain(): Promise<void> {
    actionErrorMessage = "";
    try {
      await TwitchLoginService.OpenVerificationPage();
    } catch (error) {
      actionErrorMessage = `Couldn't open your browser. Open ${loginState.verificationUri} yourself instead. (${error})`;
    }
  }

  async function cancelLogin(): Promise<void> {
    actionErrorMessage = "";
    try {
      await TwitchLoginService.CancelLogin();
    } catch (error) {
      actionErrorMessage = String(error);
    }
  }
</script>

<section class="twitch-login">
  <h1>Connect Twitch</h1>

  {#if isWaitingForApproval}
    <p class="explanation">Approve Whendo on the Twitch page that opened in your browser. Twitch will show this code:</p>
    <p class="user-code is-selectable">{loginState.userCode}</p>
    <p class="explanation">Make sure it matches, then click <strong>Activate</strong> and <strong>Authorize</strong>.</p>
    <div class="actions">
      <button type="button" class="button is-primary" onclick={openTwitchAgain}>Open Twitch again</button>
      <button type="button" class="button" onclick={cancelLogin}>Cancel</button>
    </div>
  {:else}
    <p class="explanation">{logoutExplanation}</p>
    {#if loginState.loginErrorMessage}
      <p class="details is-selectable">Details: {loginState.loginErrorMessage}</p>
    {/if}
    <div class="actions">
      <button type="button" class="button is-primary" onclick={startLogin} disabled={isStarting}>
        {isStarting ? "Contacting Twitch…" : "Log in with Twitch"}
      </button>
    </div>
  {/if}

  {#if actionErrorMessage}
    <p class="details is-selectable" role="alert">{actionErrorMessage}</p>
  {/if}
</section>

<style>
  .twitch-login {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-4);
    max-width: 36rem;
    text-align: center;
  }

  h1 {
    font-size: var(--font-size-title);
  }

  .explanation {
    color: var(--color-text-muted);
    font-size: 15px;
  }

  .user-code {
    padding: var(--space-2) var(--space-5);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-large);
    background: var(--color-surface-raised);
    font-size: 2rem;
    font-weight: 700;
    letter-spacing: 0.15em;
  }

  .actions {
    display: flex;
    gap: var(--space-2);
  }

  .button:disabled {
    cursor: progress;
  }

  .details {
    color: var(--color-text-muted);
    font-size: 13px;
  }
</style>
