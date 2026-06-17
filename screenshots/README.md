# Screenshots

Drop the following into this directory once captured. The main README
links to `grafana-dashboard.png`.

- `grafana-dashboard.png` — full "Perforce overview" dashboard, all-green
  steady state. Capture at 1600×1000 or wider.
- `grafana-dashboard-replica-down.png` *(optional)* — same dashboard with
  replica stopped so the `Targets up` stat panel shows red `DOWN` and the
  `Replication sequence lag` panel has a gap.
- `prometheus-alerts.png` *(optional)* — Prometheus → Alerts page showing
  `PerforceTargetDown` in `firing` state.

Open Grafana at <http://127.0.0.1:3000> (admin/admin), navigate to
`Dashboards → Perforce → Perforce overview`, and use the browser's
screenshot capture (or `Cmd+Shift+4` on macOS) for the full panel area.
