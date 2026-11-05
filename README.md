# k8s-resourcequota-headroom

[![CI](https://github.com/moveeeax/k8s-resourcequota-headroom/actions/workflows/ci.yml/badge.svg)](https://github.com/moveeeax/k8s-resourcequota-headroom/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/moveeeax/k8s-resourcequota-headroom)](https://goreportcard.com/report/github.com/moveeeax/k8s-resourcequota-headroom)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.26%20%7C%201.27-00ADD8.svg)](go.mod)

A `ResourceQuota` does not warn you before it bites. The first sign you are out of quota is a Pod stuck `Pending` with a `forbidden: exceeded quota` error — often mid-deploy. `k8s-resourcequota-headroom` reads every ResourceQuota and tells you which namespaces are about to hit the wall, while you can still do something about it.

For every quota it reports **used vs hard**, **percent consumed**, and a **severity** — and it can gate CI so a namespace running out of room fails the pipeline instead of failing the next rollout.

## Install

```sh
go install github.com/moveeeax/k8s-resourcequota-headroom@latest
```

Or build from source:

```sh
git clone https://github.com/moveeeax/k8s-resourcequota-headroom
cd k8s-resourcequota-headroom
go build -o k8s-resourcequota-headroom .
```

## Usage

```sh
# Every namespace, warn at 80% (default), critical at 95%
k8s-resourcequota-headroom scan --all-namespaces

# One namespace, JSON, custom thresholds
k8s-resourcequota-headroom scan -n team-a --warn 70 --critical 90 --output json

# Gate CI: exit non-zero if anything is critical
k8s-resourcequota-headroom scan --all-namespaces --fail-on critical
```

Example output (`scan --input examples/fixtures/quotas.json`):

```
NAMESPACE  QUOTA    RESOURCE         USED  HARD  PERCENT  SEVERITY
team-a     compute  pods             48    50    96.00%   CRITICAL
team-a     compute  requests.cpu     4     10    40.00%   OK
team-a     compute  requests.memory  17Gi  20Gi  85.00%   LOW HEADROOM
team-b     objects  configmaps       5     20    25.00%   OK
team-b     objects  secrets          3     20    15.00%   OK
team-b     objects  services         2     10    20.00%   OK

Summary: CRITICAL=1  LOW HEADROOM=1  OK=4
```

### Severities

| Severity       | Meaning                                   |
| -------------- | ----------------------------------------- |
| `OK`           | below `--warn`                            |
| `LOW HEADROOM` | at or above `--warn`, below `--critical`  |
| `CRITICAL`     | at or above `--critical`                  |

CPU (`m`) and memory (`Mi`/`Gi`) quantities are parsed through `resource.Quantity`, so `750m` of `1` CPU is correctly `75%` and `1536Mi` of `2Gi` is `75%`.

## Offline mode

The headroom math is a pure function over ResourceQuota objects, so it runs with no cluster access against a dump:

```sh
kubectl get resourcequota -A -o json > quotas.json
k8s-resourcequota-headroom scan --input quotas.json --output json
```

`--input` accepts a `ResourceQuotaList`, a bare array, or a single ResourceQuota, in JSON or YAML.

## CI gate

`--fail-on warn` exits non-zero when any `LOW HEADROOM` or `CRITICAL` finding exists; `--fail-on critical` only on `CRITICAL`. The process exits `2` when the gate is exceeded, `1` on error, `0` when clean.

```yaml
- name: Check quota headroom
  run: k8s-resourcequota-headroom scan --all-namespaces --fail-on critical
```

## How it works

`scan` fetches ResourceQuotas via `client-go` (or reads them from `--input`), then hands them to a pure classifier that computes `100 * used / hard` per resource and maps it to a severity. Live access and offline dumps share the same classifier, so what CI checks is exactly what the tests cover.

## License

MIT — see [LICENSE](LICENSE).
