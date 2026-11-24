# Task Manager

[![Release](https://github.com/h4r5h6l/finsec_vantage/actions/workflows/release.yml/badge.svg)](https://github.com/h4r5h6l/finsec_vantage/actions/workflows/release.yml)

A Go task manager with a JSON API, server-rendered web UI, and PostgreSQL, plus
a FinOps &amp; SecOps Portal preview. It runs with Docker Compose or Kubernetes
(Minikube), and CI/CD runs on GitHub Actions with images published to GHCR.

## Repository structure

```text
.
├── .github/          # CI, release, and Dependabot configuration
├── cmd/server/       # Task manager entry point
├── cmd/portal/       # FinOps & SecOps Portal (M1–M3 preview)
├── internal/         # Configuration, database, API, and health handlers
├── migrations/       # Embedded PostgreSQL schema migrations
├── web/              # HTML handlers, templates, and static assets
├── k8s/              # Base Kubernetes manifests + GHCR overlay
├── loadtest/         # k6 script, results, and load-test report
├── Dockerfile
├── docker-compose.yaml
├── Makefile
└── README.md
```

## CI/CD

| Workflow | Trigger | What it does |
| --- | --- | --- |
| [`ci.yml`](.github/workflows/ci.yml) | Pull requests, manual runs, and calls from Release | `gofmt`, `go vet`, `go mod tidy -diff`, race-enabled unit tests with coverage, PostgreSQL integration tests, gitleaks, govulncheck, Trivy, kustomize/kubeconform validation, and production + Portal Docker builds |
| [`release.yml`](.github/workflows/release.yml) | Push to `main`, `v*.*.*` tags, manual runs | Runs CI, builds the production image, scans it with Trivy, pushes it to GHCR, then promotes the scanned digest |

Images are published to `ghcr.io/h4r5h6l/finsec_vantage`:

- `main` publishes `latest`, `main`, and `sha-<short>`.
- A `v1.2.3` tag publishes `v1.2.3`, `1.2.3`, `1.2`, and `1`.

The pipeline pushes only the immutable full-SHA tag first. Trivy scans that
digest, and the branch/version tags are created only if the scan passes, so a
vulnerable image is never promoted. BuildKit also attaches SBOM and provenance
attestations to the image.

No extra repository secrets are needed. The workflows use the built-in
`GITHUB_TOKEN` to push to GHCR; if the push is denied, enable **Read and write
permissions** under *Settings → Actions → General → Workflow permissions*.
`GITLEAKS_LICENSE` is only required for organization-owned repositories.
Dependabot (`.github/dependabot.yml`) proposes Go, Docker, and Actions updates
weekly, and every proposal runs through the same pipeline.

Deploy the published image with the GHCR overlay:

```bash
kubectl apply -k k8s/overlays/ghcr
# or
make k8s-ghcr
```

GHCR packages are private by default. Either make the package public in its
GitHub settings or create a pull secret in the cluster and reference it from the
Deployment. Local Minikube development keeps using `make k8s-up`, which builds
and loads the local image instead of pulling from GHCR.

The project targets Go 1.26.11 (declared in `go.mod`); the Dockerfile pins the
matching `golang:1.26.11-alpine` build image.

## Run locally

```bash
docker compose up -d --build
```

Open <http://localhost:8080>. Check the app and database connection with
`curl http://localhost:8080/ready`. Stop the stack with `docker compose down`.

## Portal (M1–M3 preview)

`cmd/portal` is the FinOps & SecOps Portal service described in `port.md`.
Milestones M1 (Portal image), M2 (HTML dashboard), and M3 (health aggregation)
are implemented:

- Production image target: `make portal-image` or
  `docker build --target portal .`.
- Server-rendered dashboard at `/` using `html/template` and vendored htmx
  2.0.11 (no npm build). The license is committed next to the library.
- Liveness probe at `/health` and the `-healthcheck` flag used by the
  container `HEALTHCHECK`. Liveness never calls the task manager.
- Readiness probe at `/ready` calls the task manager's `/health` and DB-backed
  `/ready` concurrently. It returns `503` when either upstream probe fails, so
  a database outage also takes the Portal out of rotation.
- `GET /api/portal/health` returns the aggregate `portal`, `application`, and
  `database` components plus an overall `status` (`ok` or `degraded`). It
  returns `200` because it is a report, not a probe.
- Upstream probes use a two-second HTTP timeout and a three-second request
  deadline, so a hung task manager cannot stall the dashboard.
- Configuration through `PORT` (default `8080`), `TASK_MANAGER_URL` (default
  `http://localhost:8080`), and `APP_ENV`.

Run it against the task manager:

```bash
make docker-up      # task manager on :8080
make portal-run     # Portal on :8082
```

Open <http://localhost:8082>. The Portal, application, and database cards show
live status. Security scan, drift, and cost cards still report that data is
unavailable rather than faking a value; those features land in M6–M10.

Check health from the command line:

```bash
curl -s http://localhost:8082/api/portal/health | jq
curl -i http://localhost:8082/ready
```

## API

All routes are also available under `/api/v1`.

| Method | Path | Success response |
| --- | --- | --- |
| `POST` | `/tasks` | `201 Created` |
| `GET` | `/tasks` | `200 OK` |
| `GET` | `/tasks/{id}` | `200 OK` |
| `PUT` | `/tasks/{id}` | `200 OK` |
| `DELETE` | `/tasks/{id}` | `204 No Content` |

The web interface is served at `/`. Liveness is checked at `/health`; database
readiness is checked at `/ready`.

## Kubernetes

Deploy the manifests to Minikube with `make k8s-up`. The deployment starts with
one replica; the included HPA scales it from 1 to 5 based on a 70% CPU target.
Enable metrics first with `minikube addons enable metrics-server`. The HPA
percentage is CPU usage relative to the pod's 200m CPU request, so 70% equals
140m per pod. It is a scaling target, not a CPU cap.

For local access, run `make k8s-forward`, then use <http://localhost:8081>.

## Validation results

The committed k6 acceptance run used 500 virtual users for 2 minutes through
the NGINX Ingress on Minikube.

| Measure | Result |
| --- | ---: |
| Requests | 61,063 |
| Throughput | 504.6 requests/s |
| Failed requests / checks | 0 / 0 (0%) |
| p90 / p95 latency | 299.5 ms / 518.1 ms |
| Maximum latency | 1.68 s |
| HPA replicas | Scaled from 1 to 3 |

CPU was the scaling signal under this load; the available run evidence does not
include memory samples, so memory pressure cannot be ruled out independently.
The HPA trace, k6 output, and reproducible commands are in
[`loadtest/README.md`](loadtest/README.md), [`loadtest/load500-2m.txt`](loadtest/load500-2m.txt),
and [`loadtest/hpa-during-load500.txt`](loadtest/hpa-during-load500.txt).

The Kubernetes probes use `/health` for liveness and `/ready` for database
readiness. The committed evidence covers HPA scale-out; capture before/after
`kubectl get pods` output during a container restart to document the restart
count requirement.

## Checks

```bash
make fmt-check
make vet
make test
make test-race
make ci           # fmt-check + vet + test, the same checks CI runs
```

The PostgreSQL repository test is opt-in and needs a disposable database:

```bash
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/tasks?sslmode=disable' \
  make test-integration
```

CI runs it against a `postgres:16-alpine` service container. Locally, start the
database with `docker compose up -d postgres` first.
