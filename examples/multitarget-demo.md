# Demo storyboard — install & configure Signals for multiple databases

Internal recording guide for the "install and configure Elevarq Signals"
walkthrough (screenshots + a short video). This is the **local** version:
it uses the checked-in multi-target stack, so it is fully repeatable and
needs no cloud account.

> **Internal-only.** Per the media-asset policy these captures are not
> for external publication until the patent filing is done and counsel
> clears them.

## Prerequisites

- Docker (Compose v2), `helm`, `curl`, `python3`
- A screen recorder (OBS / ScreenFlow / QuickTime)

## One-command bring-up

```sh
OBS=1 examples/multitarget-snapshot.sh
```

This renders the multi-target config **from the Helm chart**, brings up two
PostgreSQL instances + one Signals collector + Prometheus + Grafana, runs a
collection, and prints the URLs. The stack is left running for recording.

- Signals API: <http://localhost:8081> (bearer `dev-local-only-replace-in-prod-32chars`)
- Prometheus: <http://localhost:9090>
- Grafana: <http://localhost:3000> (admin / admin) → dashboard *Elevarq Signal / Signal operational health*

Tear down when done: `examples/multitarget-snapshot.sh --down`

## Arc (≈2–3 min) — Configure → Install → Collect → Observe

1. **Configure (multiple databases).** Show `examples/.rendered-multitarget-signals.yaml`
   — the `targets:` list with `default` (pg-primary) and `analytics`
   (pg-analytics). Talking point: one install, several databases; only
   paths/refs/env-var names in config, never credentials.
2. **Install.** Show the bring-up (the collector + both databases starting).
   Talking point: single static, non-root container; read-only role.
3. **Collect & verify.** In a terminal:
   ```sh
   curl -s localhost:8081/status -H "Authorization: Bearer dev-local-only-replace-in-prod-32chars" | python3 -m json.tool
   ```
   Point at `target_count: 2` and both targets' `last_collected`.
4. **Observe in Grafana.** Open the dashboard. Every panel groups by the
   `target` label, so both databases appear side by side — collection
   cycles/sec by target, duration p95 by target, last-successful-collection
   freshness, collectors succeeded/failed by target. Talking point: this is
   the collector's **operational health**, not the PostgreSQL data itself
   (that lives in the snapshot).
5. **Export a snapshot** (optional closing beat):
   ```sh
   curl -s localhost:8081/export -H "Authorization: Bearer dev-local-only-replace-in-prod-32chars" -o signals-snapshot.zip
   ```

## Screenshot set (matches the #348 acceptance list)

- The multi-target config (`targets:` list)
- Install / bring-up output
- `/status` showing both databases collected (`target_count: 2`)
- Grafana dashboard with per-database panels
- A collection snapshot (optional)

## Notes

- Local convenience only: hard-coded dev token and `admin/admin` Grafana —
  never a production posture (see `docs/grafana-dashboard.md`).
- The Grafana/Prometheus assets are reused from `examples/observability/`;
  the dashboard is already per-`target`, so multiple databases show up
  with no dashboard edits.
