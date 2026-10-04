# Gul host operations

E8-T3 provides a deployment validator and a launchd plist renderer in
`internal/deployment`. They have no installation or Tailscale mutation side
effects. The shared host assembles the authenticated HTTPS listener and checks
its protected local certificate before applying the remote readiness gate.

## Remote readiness

The host first reads `tailscale version --daemon --json`. Remote admission is
limited to CLI 1.102.4 at source revision
`3caf7d9e7dcaba589cfc58beda596929733e4fea`, with an identical daemon version.
Unknown versions block remote access until their snapshot contract is checked.
The host reads `tailscale status --json` and passes those bytes to
`InspectTailscaleStatus`, with the expected node DNS name. It separately reads
`tailscale serve status --json` and passes that snapshot to
`ValidateServeStatus`. The expected endpoint is the node's exact `*.ts.net:port`
HTTPS address. The accepted root handler is a proxy to
`https+insecure://127.0.0.1:<Gul HTTPS port>` and requires the verified local
certificate. Other schemes, addresses, paths, Unix sockets, extra mounts, HTTP
ingress, and Funnel block remote readiness. The validator also rejects
foreground or Service configurations it cannot qualify and rejects another
endpoint targeting Gul's loopback port.

`ServeExpectation.Authenticated` and `LocalCertificateVerified` are host proofs,
not conclusions from Tailscale. For the installed Tailscale 1.102.4,
`serve status --json` marshals the complete `ServeConfig`, including `Services`;
the host sets `NoServiceRoutesVerified` only after the CLI/daemon version gate
and both read-only status commands succeed. A `funnel` entry in
`tailscale status --json` `.Self.CapMap` describes capability and cannot be used
to decide whether Funnel is enabled. Use Serve's `AllowFunnel` for that check.

These checks inspect configuration snapshots. They do not establish live
reachability, authenticated browser behavior, certificate pinning, or actual
provider compatibility. Local shell access may continue when remote readiness
is blocked.

## User launchd agent

`RenderLaunchdPlist` accepts the current user's verified home, an absolute Gul
binary path, and the exact data directory
`~/Library/Application Support/Gul`. It renders `gul serve --data-directory`
with that path and any selected `--port`, `--tailnet-host`,
`--dolgorae-executable`, repeated `--workspace-root` and repeated `--policy`,
using the `xyz.rootkernel.gul.serve` label. The agent starts at
login, restarts after exit with a 30-second throttle, and uses a `077` umask.
The shared host owns the singleton lock, reports port collisions, and drains on
SIGINT/SIGTERM. After wake or reconnect, refresh the browser to read current
authentication and navigation. Verify the replacement binary before loading the
agent; live provider wake/recovery remains E9-owned. Verify that the agent
can resolve the qualified `tailscale` executable in its own environment. Select
the qualified Dolgorae executable with an absolute `--dolgorae-executable` path,
or verify its lookup in the agent environment; launchd does not inherit an
interactive shell PATH.

Before rendering, create `~/Library/Logs/Gul` as the current user with mode
`0700`. Create `stdout.log` and `stderr.log` inside it as regular, single-link,
current-user-owned files with mode `0600`. The renderer checks those paths and
their parent directories, then returns plist bytes. Installation, loading,
unloading, upgrades, and log rotation belong to the host operation that uses
those bytes; the renderer performs none of them. For an upgrade, unload the
agent so `KeepAlive` cannot restart the old binary, wait for the owner to shut
down gracefully, replace and verify the binary, then load the agent again.

## Gul commands

`gul` with no subcommand opens the native desktop, starting the shared core if
needed or attaching to its verified headless owner. Host flags configure only a
new owned core; attaching leaves the existing owner's port and tailnet
configuration in effect. Initial HTTPS loading is
bounded to 30 seconds; a failed load exits with an actionable error. Trusted
`--dolgorae-executable <qualified absolute path>`, repeated
`--workspace-root <approved absolute root>` and repeated `--policy <name>`
select the released provider and its allowed launch inputs for a newly owned
core. These flags also apply to `serve` and `launchd-plist`; they do not
reconfigure an attached owner.

`gul serve --data-directory <protected absolute directory> --port 17423` starts
only the shared HTTPS host. With no `--tailnet-host`, remote origins are absent;
local Gul.app use remains available. Add `--tailnet-host <node.ts.net:443>` only
after preparing the matching Serve configuration. Startup takes a read-only
snapshot; remote requests refresh it with a bounded one-second admission cache.
A concurrent request during inspection can receive a temporary 503 and retry.
Unknown Hosts are refused. Requests bearing proxy forwarding headers cannot use
the local-only Host, even if their Host and Origin claim loopback. Those headers
only restrict routing and never grant identity. A failed or unverified snapshot blocks remote access
without stopping local HTTPS. Forwarded identity is never used for authorization.

If the provider fails its initial start, Gul keeps local presentation and files
available with runtime actions blocked and zero automatic restart attempts.
Check the selected qualified executable, private runtime directories and approved
Workspace roots. After repair, stop and restart the verified Gul host owner to
retry; closing an attached desktop window does not stop a headless owner. An
installed launchd agent must be unloaded before stopping its owner, then loaded
again after repair. A provider that has already started uses the bounded crash
restart policy; after its restart budget is exhausted, use the same owner restart
procedure. Inspect a reported socket collision or unsafe cleanup separately;
never delete another process's socket to force admission.

`gul diagnose --data-directory <directory> --tailnet-host <node.ts.net:443>`
verifies the private local owner with a fresh nonce and certificate pin, then
refreshes node and Serve diagnostics. It mints no setup grant and makes no Serve
change. `gul launchd-plist` writes only the rendered XML to stdout after checking
the protected log paths; loading that file is a separate host operation.

SIGINT or SIGTERM closes the desktop and drains a core owned by that process,
or drains a headless host. A failed owned
lifecycle stop retains its lock for retry. After wake or connection loss, browser refresh performs fresh
session and navigation reads; production provider
wake/recovery qualification remains E9-owned. An attached Gul.app window does not
stop the headless owner when it closes.

The [Tailscale 1.102.4 proxy source](https://github.com/tailscale/tailscale/blob/v1.102.4/ipn/ipnlocal/serve.go)
retains Host for TCP/HTTPS backends and overwrites `X-Forwarded-Host`,
`X-Forwarded-Proto`, and the source-address `X-Forwarded-For` headers. Its [status implementation](https://github.com/tailscale/tailscale/blob/v1.102.4/cmd/tailscale/cli/serve_legacy.go)
serializes ServeConfig, including the Service routes checked here. These source
checks explain the transport contract; they do not qualify a live deployment.

## Expired or damaged local certificate

The host refuses an expired or damaged `localhost.pem`; it does not replace that
file automatically. Stop Gul.app and the verified `gul serve` owner. If a user
agent is installed, unload it first so KeepAlive cannot restart the owner. Confirm
the owning process has exited before touching certificate state; a failed health
probe alone does not prove that the ownership lock has been released.

In the protected data directory, move `localhost.pem` to a unique private backup
name without replacing an existing backup. Keep the directory at `0700` and the
backup at `0600`. Start the verified Gul binary again: with no certificate file,
the sole owner creates a new private key and loopback leaf under its lock. Keep
`gul.sqlite`, `core.lock` and the owner record in place. Reopen the native shell
so it pins the new leaf; refresh browser connections and requalify any deployment
certificate configuration separately. This procedure does not install system
trust or alter Tailscale configuration.

## Isolated assembled verification

The E14 acceptance driver is a development fixture. Run the commands in
[TESTING.md](../TESTING.md#assembled-application-acceptance) to exercise the real
host and checked bundle with explicit stateful fakes. They allocate temporary
Gul data, Workspace and client state and remove only their own fixture state.
Production `gul` and `gul serve` assemble the configured released-provider
gateway and protected Controller store. Runtime gates remain unavailable until
the provider passes admission. E2 qualifies this assembly with the published
Dolgorae release and permitted native fakes. E14 does not install a provider,
create a live Controller, load launchd, expose Tailscale or qualify a supported
device. Live, device, fault/security and deployment qualification remain E9-owned.
