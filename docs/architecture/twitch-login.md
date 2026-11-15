# Twitch Login: How the Code Works

- **Last updated:** 2026-11-15

This page explains how Whendo's Twitch login code fits together: which parts exist, what each public function does, and how they work together for logging in, staying logged in and logging out. It's for developers working on the code. For what the login must do and why, see the design doc, [Twitch Login](../design/twitch-login.md).

## The Pieces

```mermaid
flowchart TD
    subgraph Frontend["Frontend (Svelte)"]
        App["App.svelte<br/>shows login prompt or logged-in screen"]
        LoginView["TwitchLogin.svelte<br/>login button, code, cancel"]
    end

    subgraph WailsLayer["Wails layer (package main)"]
        Service["TwitchLoginService<br/>twitchloginservice.go"]
        Config["config.go<br/>client ID, timeouts"]
    end

    subgraph Engine["Engine (plain Go, no Wails)"]
        Session["twitchauth.Session<br/>login state, tokens, refresh, validation"]
        Client["twitchauth.Client<br/>HTTP calls to Twitch"]
        Store["credentials.Store<br/>interface"]
        Keyring["credentials.KeyringStore<br/>go-keyring"]
    end

    Twitch[("Twitch<br/>id.twitch.tv")]
    OS[("OS credential store<br/>Windows Credential Manager,<br/>macOS Keychain, Secret Service")]

    App --> LoginView
    App -- "generated bindings" --> Service
    LoginView -- "generated bindings" --> Service
    Service -- "twitch-login-state-changed event" --> App
    Service --> Session
    Config --> Service
    Session --> Client
    Session --> Store
    Store -.implemented by.-> Keyring
    Client --> Twitch
    Keyring --> OS
```

| Piece | File | Job |
|---|---|---|
| `twitchauth.Client` | `engine/twitchauth/client.go` | Makes the HTTP calls to Twitch's OAuth server. Knows nothing about login state. |
| `twitchauth.Session` | `engine/twitchauth/session.go` | Owns the login: its state, the tokens, refreshing, validation and storage. The rest of the engine asks it for tokens. |
| `credentials.Store` | `engine/credentials/credentials.go` | A small interface for saving, loading and deleting secrets, so go-keyring can be replaced without changes elsewhere. |
| `credentials.KeyringStore` | `engine/credentials/credentials.go` | The `Store` backed by the OS credential store, through go-keyring. |
| `TwitchLoginService` | `twitchloginservice.go` | The Wails service the UI calls. Starts the session, opens the browser and forwards state changes. |
| `config.go` | `config.go` | The Twitch client ID, overridable with `WHENDO_TWITCH_CLIENT_ID`, plus the request timeout and the credential service name. |
| `App.svelte`, `TwitchLogin.svelte` | `frontend/src/` | Show the login prompt, the code while waiting, and a placeholder screen once logged in. |

The engine pieces never import Wails. `main.go` wires them together: it creates the `Client`, the `KeyringStore` and the `Session`, and passes the session to `TwitchLoginService`.

## Login States

The session is always in one of three states, reported as a `LoginState`:

```mermaid
stateDiagram-v2
    [*] --> LoggedOut
    LoggedOut --> LoggedIn: Restore finds a stored login
    LoggedOut --> WaitingForApproval: StartLogin
    WaitingForApproval --> LoggedIn: user approves on Twitch
    WaitingForApproval --> LoggedOut: CancelLogin
    WaitingForApproval --> LoggedOut: code expires (reason codeExpired)
    WaitingForApproval --> LoggedOut: something fails (reason loginFailed)
    LoggedIn --> LoggedOut: Logout
    LoggedIn --> LoggedOut: Twitch rejects the refresh (reason sessionEnded)
```

`LoginState` carries what the UI needs for each state:

| Status | Fields that are set |
|---|---|
| `loggedOut` | `logoutReason` (empty, `codeExpired`, `sessionEnded` or `loginFailed`) and, for `loginFailed`, `loginErrorMessage` |
| `waitingForApproval` | `userCode` and `verificationUri` |
| `loggedIn` | `accountLogin`, the Twitch username |

Every change is reported through the session's `OnStateChange` callback. `main.go` turns it into the `twitch-login-state-changed` Wails event, which the frontend listens for.

## Session Functions

| Function | What it does |
|---|---|
| `NewSession(config)` | Creates a logged-out session. |
| `State()` | Returns a copy of the current `LoginState`. |
| `Restore()` | Loads a login saved by an earlier run. Doesn't contact Twitch. |
| `StartLogin(ctx)` | Gets a code from Twitch and starts waiting for approval in the background. Refuses if already logged in. |
| `CancelLogin()` | Stops waiting for approval. |
| `Logout(ctx)` | Forgets the login and tells Twitch to revoke the token. |
| `AccessToken(ctx)` | Returns a valid access token, refreshing first if it's about to expire. This is what the rest of the engine calls. |
| `RenewAccessToken(ctx, rejectedToken)` | Gets a new token after Twitch rejected one. Concurrent callers share a single refresh. |
| `AccountUserID()` | Returns the logged-in account's Twitch user ID. |
| `KeepValidated(ctx)` | Checks the login with Twitch immediately and then every hour, until `ctx` is cancelled. |

`TwitchLoginService` exposes a subset to the UI: `GetLoginState`, `StartLogin` (which also opens the browser), `OpenVerificationPage`, `CancelLogin` and `Logout`.

## Startup

```mermaid
sequenceDiagram
    participant Wails
    participant Service as TwitchLoginService
    participant Session
    participant Store as Credential store
    participant Twitch

    Wails->>Service: ServiceStartup(ctx)
    Service->>Session: Restore()
    Session->>Store: Get the four stored entries
    alt all four found
        Session->>Session: state = loggedIn
    else any missing
        Session->>Session: stay loggedOut
    end
    Service-)Session: go KeepValidated(ctx)
    Session->>Twitch: validate the token right away
    Note over Session,Twitch: then every hour (see Hourly Validation)
```

The frontend subscribes to `twitch-login-state-changed` first and then calls `GetLoginState`, so it can't miss a change that happens in between.

## Logging In

Whendo uses Twitch's Device Code Grant flow: Twitch gives Whendo a code, the user approves it in their browser, and Whendo polls until the approval arrives.

```mermaid
sequenceDiagram
    actor User
    participant UI as Frontend
    participant Service as TwitchLoginService
    participant Session
    participant Twitch
    participant Store as Credential store

    User->>UI: clicks "Log in with Twitch"
    UI->>Service: StartLogin()
    Service->>Session: StartLogin(ctx)
    Session->>Twitch: POST /oauth2/device (client ID, scopes)
    Twitch-->>Session: device code, user code, verification link
    Session->>Session: state = waitingForApproval
    Session-)Session: poll in the background
    Session-->>Service: state with the code and link
    Service->>User: opens the verification link in the browser
    Service-->>UI: state (UI shows the code)

    loop every few seconds until approved or expired
        Session->>Twitch: POST /oauth2/token (device code)
        Twitch-->>Session: authorization_pending
    end
    User->>Twitch: approves Whendo
    Session->>Twitch: POST /oauth2/token (device code)
    Twitch-->>Session: access token and refresh token
    Session->>Twitch: GET /oauth2/validate
    Twitch-->>Session: username and user ID
    Session->>Store: save tokens, username and user ID
    Session->>Session: state = loggedIn
    Session-->>UI: twitch-login-state-changed (loggedIn)
```

Details worth knowing:
- **The background polling isn't tied to the call that started it.** Wails cancels a call's context when the call returns, so the polling runs on a context that only `CancelLogin`, `Logout` or a new `StartLogin` cancels.
- **If the code expires,** the state becomes `loggedOut` with reason `codeExpired`, and the UI offers to start again.
- **If saving the login fails,** anything partly saved is removed and the state becomes `loggedOut` with reason `loginFailed`.

## Getting and Refreshing Tokens

Code that calls Twitch's APIs gets its token from the session:

```mermaid
sequenceDiagram
    participant Caller as Engine code (for example EventSub)
    participant Session
    participant Twitch
    participant Store as Credential store

    Caller->>Session: AccessToken(ctx)
    alt token valid for more than 5 minutes
        Session-->>Caller: current token
    else about to expire
        Session->>Twitch: refresh (see below)
        Session-->>Caller: new token
    end

    Caller->>Twitch: API call with the token
    Twitch-->>Caller: 401 rejected
    Caller->>Session: RenewAccessToken(ctx, rejected token)
    Session->>Twitch: POST /oauth2/token (refresh token)
    Twitch-->>Session: new access and refresh tokens
    Session->>Store: save refresh token, then access token
    Session-->>Caller: new token
```

**Only one refresh runs at a time.** Each refresh token works only once, so two refreshes at the same moment would invalidate each other. The session handles this:
1. A refresh holds a dedicated lock, so other refreshes wait.
2. When a waiting caller gets the lock, it compares the token it was given with the current one. If they differ, someone else already refreshed, so it returns the current token without calling Twitch.

So if five callers all report the same rejected token, Twitch sees one refresh and all five get the new token.

**If Twitch rejects the refresh,** the user changed their password, disconnected Whendo, or didn't use it for 30 days. The session logs out with reason `sessionEnded`, removes the stored login and returns `ErrNotLoggedIn`.

**The refresh token is saved first,** because the old one stops working the moment Twitch issues a new one. If saving fails, the new tokens are kept in memory, so the user stays logged in until Whendo closes.

## Hourly Validation

Twitch requires apps to check their token at startup and every hour after. `KeepValidated` does this:

```mermaid
flowchart TD
    Start([validate]) --> LoggedIn{logged in?}
    LoggedIn -- no --> WaitHour[wait an hour]
    LoggedIn -- yes --> Validate[GET /oauth2/validate]
    Validate -- 200 --> Update[update the expiry time,<br/>and the username if it changed]
    Update --> WaitHour
    Validate -- 401 --> Refresh[refresh the token]
    Refresh -- succeeded --> WaitHour
    Refresh -- rejected --> LogOut[log out, reason sessionEnded]
    LogOut --> WaitHour
    Validate -- network error --> WaitMinute[wait a minute]
    Refresh -- network error --> WaitMinute
    WaitHour --> Start
    WaitMinute --> Start
```

A network error doesn't log the user out. Validation just tries again sooner, so being offline briefly never costs the user their login.

## Logging Out

```mermaid
sequenceDiagram
    actor User
    participant UI as Frontend
    participant Service as TwitchLoginService
    participant Session
    participant Store as Credential store
    participant Twitch

    User->>UI: clicks "Log out"
    UI->>Service: Logout()
    Service->>Session: Logout(ctx)
    Session->>Session: stop any login in progress
    Session->>Session: state = loggedOut
    Session->>Store: delete the four entries
    Session->>Twitch: POST /oauth2/revoke
    Session-->>UI: twitch-login-state-changed (loggedOut)
```

The login is forgotten locally **before** Twitch is told. If Twitch can't be reached, the user is still logged out; the failure is only logged, and the token expires on its own.

## Discarding Stale Results

Network calls happen without holding the session's lock, so the login can change while one is in progress. For example, the user might log out while a refresh is running.

To handle this, the session keeps a `generation` number that increases on every status change: login, logout, cancel and Twitch ending the session. Before a network call, the code notes the generation. Afterwards, it checks whether the generation is still the same, and throws its result away if it isn't. This stops, for example:
- a refresh that finishes after logout from saving tokens and logging the user back in
- a cancelled login attempt from logging the user in
- an old attempt's "code expired" from replacing a newer attempt's code

A simple status check wouldn't be enough, because the status can return to the same value: a user can log out and back in during a single refresh. The generation number never repeats.

## What's Stored

Each item is its own entry in the OS credential store, under the service name `Whendo`. Windows Credential Manager limits the size of a single entry, so the items aren't bundled into one.

| Entry | Contents |
|---|---|
| `twitch-access-token` | The current access token |
| `twitch-refresh-token` | The current refresh token |
| `twitch-account-login` | The Twitch username, kept so the UI can show it before the first validation |
| `twitch-account-user-id` | The Twitch user ID |

`Restore` only treats the login as present when all four exist.

## Using Your Own Twitch Application

Set `WHENDO_TWITCH_CLIENT_ID` before starting Whendo to log in through your own Twitch application. It must use the **Public** client type. In PowerShell:

```powershell
$env:WHENDO_TWITCH_CLIENT_ID = "<your client ID>"; wails3 dev
```

Logins aren't stored separately for each application, so switching between applications asks you to log in again.

## Tests

`engine/twitchauth/twitchauth_test.go` runs the client and session against a fake Twitch server built with `httptest`, with an in-memory `credentials.Store`. The tests cover logging in, an expired code, cancelling, a single refresh for concurrent callers, logging out when a refresh is rejected, hourly validation refreshing a rejected token, and logout. Run them with:

```sh
go test -race ./engine/...
```
