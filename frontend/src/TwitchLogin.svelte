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
  <h1 class="title">Connect Twitch</h1>

  {#if isWaitingForApproval}
    <p class="subtitle">Approve Whendo on the Twitch page that opened in your browser. Twitch will show this code:</p>
    <p class="user-code">{loginState.userCode}</p>
    <p class="subtitle">Make sure it matches, then click <strong>Activate</strong> and <strong>Authorize</strong>.</p>
    <div class="actions">
      <button class="btn" onclick={openTwitchAgain}>Open Twitch again</button>
      <button class="btn secondary" onclick={cancelLogin}>Cancel</button>
    </div>
  {:else}
    <p class="subtitle">{logoutExplanation}</p>
    {#if loginState.loginErrorMessage}
      <p class="details">Details: {loginState.loginErrorMessage}</p>
    {/if}
    <div class="actions">
      <button class="btn" onclick={startLogin} disabled={isStarting}>
        {isStarting ? "Contacting Twitch…" : "Log in with Twitch"}
      </button>
    </div>
  {/if}

  {#if actionErrorMessage}
    <p class="details" role="alert">{actionErrorMessage}</p>
  {/if}
</section>

<style>
  .twitch-login {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--s-2);
    max-width: 36rem;
    text-align: center;
  }

  .user-code {
    margin: 0;
    padding: var(--s-1) var(--s-3);
    border: 1px solid var(--glass-border);
    border-radius: var(--radius);
    background: var(--glass-strong);
    font-size: 2rem;
    font-weight: 700;
    letter-spacing: 0.15em;
    -webkit-user-select: text;
    user-select: text;
  }

  .actions {
    display: flex;
    gap: var(--s-1);
    --wails-draggable: no-drag;
  }

  .btn:disabled {
    opacity: 0.6;
    cursor: progress;
  }

  .secondary {
    background: var(--glass-strong);
    border: 1px solid var(--glass-border);
  }

  .details {
    margin: 0;
    color: var(--muted);
    font-size: 0.85rem;
    -webkit-user-select: text;
    user-select: text;
  }
</style>
