<script lang="ts">
  import {Events} from "@wailsio/runtime";
  import {TwitchLoginStateChanged} from "../bindings/events";
  import {TwitchLoginService} from "../bindings/github.com/SloanGwaltney/whendo";
  import {LoginStatus} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchauth";
  import type {LoginState} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchauth";
  import TwitchLogin from "./TwitchLogin.svelte";

  let loginState: LoginState | null = $state(null);
  let logoutErrorMessage = $state("");

  $effect(() => {
    // Subscribe before asking for the current state, so a change that
    // happens in between isn't missed.
    const unsubscribe = Events.On(TwitchLoginStateChanged, (event) => {
      loginState = event.data;
    });
    TwitchLoginService.GetLoginState()
      .then((currentState) => {
        loginState ??= currentState;
      })
      .catch((error: unknown) => console.error("Couldn't load the Twitch login state", error));
    return unsubscribe;
  });

  async function logOut(): Promise<void> {
    logoutErrorMessage = "";
    try {
      await TwitchLoginService.Logout();
    } catch (error) {
      logoutErrorMessage = String(error);
    }
  }
</script>

<main class="container">
  {#if loginState === null}
    <p class="subtitle">Loading…</p>
  {:else if loginState.status === LoginStatus.LoginStatusLoggedIn}
    <!-- Placeholder until the Home and Settings screens exist. -->
    <h1 class="title">Logged in as {loginState.accountLogin}</h1>
    <button class="btn" onclick={logOut}>Log out</button>
    {#if logoutErrorMessage}
      <p class="subtitle">Logged out, but something went wrong: {logoutErrorMessage}</p>
    {/if}
  {:else}
    <TwitchLogin {loginState}/>
  {/if}
</main>
