# service-template

A template for a service on the [homelab platform](https://github.com/Karppimc/homelab-platform): a small Go HTTP service with a Dockerfile, a CI pipeline and the one values file the platform needs to run it. A service created from it starts on the paved road: it passes the platform's policies, is scraped by Prometheus and deploys through Argo CD without writing any Kubernetes manifests.

## What you get

| Piece | What it does |
|---|---|
| `main.go` | HTTP server with `/healthz` (liveness), `/readyz` (readiness), `/metrics` (Prometheus), JSON logs and a graceful shutdown on SIGTERM |
| `main_test.go` | Tests for the endpoints and the request metrics |
| `Dockerfile` | Multi-stage build to a distroless image: static binary, numeric non-root user, base images pinned by digest |
| `deploy/values.yaml` | Values for the platform's service chart: name, image, port, replicas, resources |
| `.github/workflows/ci.yml` | Test, build, smoke test, vulnerability scan, platform check, and push to GHCR on `main` |

The Deployment, Service and ServiceMonitor are not in this repository. The platform's chart renders them from `deploy/values.yaml` with the labels, security context and probes its policies ask for.

## Create a service

1. **Create the repository:** "Use this template" on GitHub, or `gh repo create <owner>/<name> --template Karppimc/service-template --public`.
2. **Name it** in `deploy/values.yaml`: `name`, `image.repository` (`ghcr.io/<owner>/<name>`, lowercase) and `env.SERVICE_NAME`.
3. **Publish it, if it needs to be reachable from the LAN:** uncomment `ingress.host` in `deploy/values.yaml` and choose a `*.home.arpa` name. The platform adds an HTTPS-only Ingress with a certificate.
4. **Write the service:** replace the index handler in `main.go` with your routes. Keep `/healthz`, `/readyz` and `/metrics`: the platform's probes and Prometheus depend on them.
5. **Push to `main`** through a pull request. CI pushes the image `ghcr.io/<owner>/<name>:<commit SHA>`. The first time, make the package public under the repository's Packages settings, so the cluster can pull it without a secret.
6. **Release:** set `image.tag` in `deploy/values.yaml` to the commit SHA of the image, in a pull request.
7. **Onboard to the platform** with two files in homelab-platform, described in its README under "Onboarding a workload": the workload file (namespace, quota, Argo CD project) and an Application that combines the platform's chart with this repository's `deploy/values.yaml`.

After that, a new version is one pull request here that changes `image.tag`.

## CI

| Job | What it checks |
|---|---|
| `test` | `gofmt`, `go vet`, `go test` |
| `image` | Builds the image, runs it read-only as a non-root user without capabilities and calls the endpoints, scans it with Trivy (fails on HIGH and CRITICAL vulnerabilities that have a fix), and on `main` pushes it tagged with the commit SHA |
| `deploy` | Runs the platform's own check on `deploy/values.yaml`: the chart's schema, the Kubernetes schemas and every Kyverno policy |
| `validate` | Passes only when the three jobs above passed; the one check the branch ruleset requires |

Actions are pinned by commit SHA and binaries by SHA256, in the `env` block of the workflow. The image tag is the commit SHA, so a tag always means the same image.

## Run it locally

```bash
go test ./...
go run .                      # http://localhost:8080

docker build --build-arg VERSION=dev -t service-template:dev .
docker run --rm --read-only --cap-drop ALL --user 65532:65532 -p 8080:8080 service-template:dev
```

## What the platform expects from a service

- It listens on one port (`PORT`, default 8080) and serves `/healthz`, `/readyz` and `/metrics` there.
- It runs as a non-root user with a read-only root filesystem: write only to volumes you declare.
- It stops within 20 seconds of SIGTERM.
- It logs to stdout; the platform collects the logs.
- Traffic out of its namespace is denied except DNS. A service that calls something outside adds a NetworkPolicy for it.
