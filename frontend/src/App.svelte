<script lang="ts">
  import {Events} from "@wailsio/runtime";
  import {TwitchLoginStateChanged} from "../bindings/events";
  import {TwitchLoginService} from "../bindings/github.com/SloanGwaltney/whendo";
  import {LoginStatus} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchauth";
  import type {LoginState} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchauth";
  import AppShell from "./AppShell.svelte";
  import TwitchLogin from "./TwitchLogin.svelte";
  import Wordmark from "./Wordmark.svelte";

  let loginState: LoginState | null = $state(null);

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
</script>

{#if loginState === null}
  <!-- Loading takes a moment at most, so there's nothing to show. -->
{:else if loginState.status === LoginStatus.LoginStatusLoggedIn}
  <AppShell accountLogin={loginState.accountLogin}/>
{:else}
  <!-- Until the user logs in, there's nothing else Whendo can do. See
       docs/design/mvp-ui.md#first-launch. -->
  <div class="first-launch">
    <Wordmark/>
    <TwitchLogin {loginState}/>
  </div>
{/if}

<style>
  .first-launch {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-6);
    padding: var(--space-6);
  }
</style>
