# service-template

Template for a Go service on the homelab platform (Karppimc/homelab-platform): code skeleton, Dockerfile, CI and the values file for the platform's service chart.

Golden-path rules and skills come from the golden-path plugin; this file only adds what is unique to this repo.

## Layout
- `main.go`, `main_test.go`: the service and its tests (one package, module name `service`)
- `Dockerfile`: multi-stage build to a distroless image, base images pinned by digest
- `deploy/values.yaml`: values for `platform/service-chart` in homelab-platform; Argo CD deploys it
- `.github/workflows/ci.yml`: test, image (build, smoke test, Trivy scan, push on main), deploy (the platform's check), validate

## Notes
- No Kubernetes manifests live here. Labels, security context, probes and the ServiceMonitor come from the platform's chart; change the chart in homelab-platform, not this repo, when a policy changes.
- Keep `/healthz`, `/readyz` and `/metrics` on the service port: the platform's probes and Prometheus depend on them.
- The container runs with a read-only root filesystem as user 65532; do not write outside declared volumes.
- The image tag is the commit SHA. A release is a pull request that changes `image.tag` in `deploy/values.yaml` to a commit whose image CI has pushed.
- Versions are pinned in the `env` block of the workflow (actions by commit SHA, binaries by SHA256) and in the Dockerfile (digests). The platform tool versions there should match homelab-platform's `validate.yml`.
- Checks before a PR: `gofmt -l .`, `go vet ./...`, `go test ./...`, `yamllint .`, `docker build .`.
- This is a template repository: everything here is copied into every service created from it. Keep it small.
