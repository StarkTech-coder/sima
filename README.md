# S.I.M.A. — Three-AUV exploration simulator

An offline-capable presentation prototype, not real vehicle control. Three independent simulated AUVs communicate with one coordinator over **real loopback UDP using AEGIS**. The embedded dashboard has no CDN, map service, frontend framework, or external runtime service.

## Run

Go **1.26.2 or newer** is required. From the repository root:

```sh
go run ./cmd/sima-sim
```

Open **http://127.0.0.1:8080**. An alternate loopback address can be selected with `-http 127.0.0.1:8081`. The simulator obtains temporary OS-selected UDP ports. A port bind conflict is a startup error; no external ports are exposed. Stop with Ctrl+C. Restarting starts a new, empty simulation session.

The first build downloads Go dependencies; subsequent execution of a prebuilt binary requires no internet. To prepare an offline demonstration, build with `go build -o sima-sim ./cmd/sima-sim` on the target platform while dependencies are available. All dashboard assets are embedded.

## Demonstrate

1. Wait until initial assignment receipts are confirmed and the control buttons become active.
2. AUV-01, AUV-02, AUV-03 execute S-01, S-02, S-03 respectively. AUV-02 also accepts S-04 **queued but not authorized to start**.
3. Click **Bağlantıyı kes**. Wait at least three seconds. Heartbeat changes to unreachable; old telemetry is explicitly marked as last known, not live. Other AUVs continue.
4. AUV-02 independently continues only its already authorized segment. Queued work never starts automatically. The center keeps ownership of uncertain work.
5. Click **Yeniden bağla**. The center obtains an actual UDP snapshot, reconciles progress, and explicitly releases the unstarted S-04 because AUV-02's energy is no longer sufficient.
6. Only after the release receipt is verified is S-04 assigned to another eligible AUV with **generation 2**. It starts after that AUV finishes its active segment. Completion results are explicitly acknowledged back to the vehicle.

**Deliberate scenario input:** the first disconnection debits AUV-02 by 12 battery percentage points as a local contingency reserve cost. This is not a physical battery model. The queued follow-up requires 80%; AUV-02 starts at 86%. This makes the release/reallocation scenario reproducible. Subsequent toggles do not debit the reserve again. S-04 generation 1 intentionally waits for this scenario; without a disconnection it stays queued.

Primary segments last about 110, 125, and 140 seconds. The follow-up lasts 55 seconds. Refreshing the browser does not reset simulation state. Restart the application for a fresh demo.

## Boundaries

- `internal/model`, `fleet`, `mission`: transport-independent state, freshness, eligibility, ownership and generation rules.
- `internal/simulation`: independent actor tickers, bounded onboard tasks, local segment execution and low-energy hold.
- `internal/coordination`: single central decision owner, stable task/vehicle ordering, bounded command retries and reconciliation.
- `internal/adapter/aegis`: the **only** package importing the external AEGIS SDK; application envelopes, client/server composition, concrete heartbeat probe. No AEGIS framing/socket transport is reimplemented.
- `internal/app`: composition, worker shutdown, explicit simulation fault gate. The coordinator never reads simulator state directly.
- `internal/web`: embedded local UI and HTTP API. The chart and contours are synthetic, not real maps or sonar.

AEGIS is pinned to `ebbaf57baefd9bfa9acedba74100df05c044e0c9` (`v0.0.0-20261109105421-ebbaf57baefd`). At this commit the canonical module is `github.com/StarkTech-coder/aegis`; no replacement is needed. The previous architecture report's module-path issue and leader-startup lock issue were corrected upstream. The prototype does not use leader election or failover.

## Safety and resource contracts

- Transport send success is **not** task acceptance; command receipts carry immutable command IDs, task IDs and assignment generations.
- Lost replies reuse the same command identity. Vehicles cache at most 128 commands, reject changed content under an existing ID, and never evict history to silently allow replay. The full cache rejects new commands.
- At most 16 tasks per vehicle, 16 route points per task, 8 KiB application messages, 160 central events, 3 concurrent poll workers, and 12 attempts per pending command. Unknown outcomes keep task reservations, including after retry exhaustion.
- Each exchange has a 250 ms deadline; central polling/command contexts are bounded to 300 ms. Heartbeat uses a 500 ms interval and two misses for unreachable. Telemetry older than 3 s is not allocation-eligible.
- Epoch changes are not silently accepted. New vehicle processes require a new demo session; stale epochs never receive commands.
- No automatic transfer of executing work. Only an explicit release receipt for the exact owner/generation frees unstarted work.
- In-memory only: there is **no durable restart recovery**, authenticated network identity, acoustic modem, DTN, ROS 2, real vehicle/sonar interface, collision avoidance, or multi-center consensus.
- UDP access is intentionally loopback-only. JSON identity/correlation validation is not cryptographic authentication. This is not a production safety controller.

## Checks

```sh
gofmt -w cmd internal
go test ./...
go test -race ./...
go vet ./...
```

Tests include deterministic allocation/ownership, stale observations, local disconnected continuation, application command replay, real UDP response loss, heartbeat transitions, three-AUV UDP reconnection and release/reallocation, result acknowledgments, bounded shutdown, and HTTP controls. Network tests use readiness round trips/condition deadlines; domain tests advance time explicitly.