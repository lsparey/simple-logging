# Changelog

All notable changes to simple-logging. From v1.0.0 on, each release's entry is written by the Release workflow from its release notes; see [CONTRIBUTING.md](CONTRIBUTING.md#releases). Versions before v0.6.0 were early development releases of the chart only.

## 1.0.0 (2026-09-24)

- Phase 6: v1.0 — docs, CI, shim removal, and fixes from a Phase 0–5 audit (#27) (`0ff03a6`)
- Phase 5: self-metrics and authentication (#26) (`b8ef2b6`)

## 0.14.1 (2026-09-24)

- Sidebar: default namespace first and pre-expanded at / (#25) (`f523946`)

## 0.14.0 (2026-09-24)

- ci: build multi-arch images without QEMU (#24) (`9cfdc87`)
- Phase 4: single Go binary, connect-go, hardened image and chart (#23) (`9361687`)

## 0.13.0 (2026-09-24)

- Search: move server-side search to its own page (#22) (`f27ca2a`)
- Sidebar: one namespace/kind/Indexes accordion (#21) (`691b285`)
- Phase 3.4: search and download frontend (#20) (`a489ab2`)
- Phase 3.2 + 3.3: log download endpoint and namespace-wide live tail (#19) (`b830089`)
- Phase 3.1: server-side SearchLogs RPC (#18) (`7c16b1c`)
- Phase 2.3 frontend: workload-grouped sidebar and log view (#17) (`a4ee975`)
- feat: workload API (ListWorkloads/GetWorkloadLogs/StreamWorkloadLogs) (Phase 2.3 backend) (#16) (`d85b0e1`)
- feat: resolve and persist workload owner from ownerReferences (Phase 2.2) (#15) (`a96d053`)
- feat: collect every container in a pod (Phase 2.1) (#14) (`6ee07e9`)

## 0.12.0 (2026-09-22)

- feat: frontend polish for segmented storage (Phase 1.8) (#13) (`f60dcf0`)
- feat: PVC-full disk guard and write resilience (Phase 1.4) (#12) (`48009a0`)
- feat: segmented log storage layout (Phase 1) (#11) (`24c55af`)
- feat: use real log-line timestamps instead of receipt wall-clock time (#10) (`1f7e723`)
- feat: Phase 0 quick wins from the improvement plan (#9) (`5212b12`)

## 0.11.2 (2026-09-22)

- fix: use Recreate strategy to avoid RWO volume conflicts on rollout (`c971690`)

## 0.11.1 (2026-09-20)

- docs: mit license (`6601bbc`)
- feat: hybrid collection mode (`7981eec`)
- fix: copy in insecure contexts (`1f67200`)

## 0.11.0 (2026-08-27)

- feat: improve log file efficiency (`11e6918`)
- feat: improve index format (`e936a77`)

## 0.10.7 (2026-08-26)

- feat: improve memory performance (`693f9bd`)
- feat: show index file count (`7cfe9d5`)
- feat: improve release action (`6f03797`)

## 0.10.6 (2026-08-12)

- fix: health check (`ba6dc6d`)

## 0.10.5 (2026-07-30)

- fix: playwright pipeline (`1118cc6`)
- feat: mobile menu (`85a0f8f`)

## 0.10.4 (2026-07-30)

- feat: view full log message (`fab8341`)
- feat: limit files list to 50 items (`8787a31`)
- feat: arm64 image build (`aa7bbb5`)

## 0.10.3 (2026-07-13)

- feat: nginx config for backend (`8201d85`)

## 0.10.2 (2026-07-10)

- fix: helm image version (`34ac0b0`)

## 0.10.1 (2026-07-10)

- fix: remove default endpoint urls (`4609079`)

## 0.10.0 (2026-06-10)

- Paginate index values by latest activity (#7) (`3493d9f`)
- fix: order index logs by timestamp (#6) (`8aeddfa`)
- feat: format JSON logs in indexes (#5) (`c405915`)
- Tolerate startup lines when detecting JSON logs (#4) (`decfb95`)

## 0.9.0 (2026-06-08)

- Add log data dashboard (#3) (`2a3fd3b`)

## 0.8.0 (2026-06-05)

- chore: admin bypass token (`cdf4543`)
- feat: allow release action to push to repo (#2) (`c087ffc`)
- Add JSON log indexes (#1) (`d89a3f1`)
- feat: histogram time marker (`9a691e4`)

## 0.7.0 (2026-06-03)

- feat: namespace path (`60a7f52`)
- feat: paging number (`c362aa2`)

## 0.6.0 (2026-06-03)

- build: helm release action (`3faa6ab`)
- release: v0.5.0 (`34a7638`)
- release: v0.3.0 (`2949b0f`)
- build: release action (`38b7632`)
- feat: resource paths (`9d6957f`)
- docs: update readme (`d58bfae`)
- feat: support file tail log collection (`00f2e6f`)
- test: run tests on pre push (`6d13240`)
- feat: histogram (`8400e31`)
- style: json chip (`7975ac8`)
- test: test on commit (`9495815`)
- feat: add timestamp to json log formatter (`d65acc3`)
- feat: format json logs (`2b01509`)
- feat: detect json log format (`515bce6`)
- fix: dockerfiles (`8d76514`)
- test: add unit tests (`4932469`)
- test: fix playwright (`e932205`)
- fix: deployment live streaming (`fbe5686`)
- test: vitest ignore e2e (`fde351f`)
- build: run tests on build and PR (`fec3297`)
- test: add paging test (`cab787a`)
- test: add e2e frontend tests (`09cff4d`)
- feat: show latest logs first (`bd7e2c5`)
- fix: add latest tag to image (`578aaf3`)
- chore: bump chart version (`8b5d02b`)
- feat: combined image (`f50e196`)
- fix: helm release (`0b7e5fb`)
- fix: helm release (`52d58e8`)
- feat: helm charts (`5c67db6`)
- build: add workflows (`892063f`)
- docs: update readme with stack info (`a68c831`)
- docs: update with logo (`cb077bf`)
- docs: add readme (`aae27c1`)
- fix: live updates (`23c9336`)
- feat: dynamic pagination (`33662dc`)
- lint: linting fixes (`0f3e891`)
- fix: searching (`127dfb9`)
- feat: styling improvements (`8501a88`)
- feat: coloured pod names (`bdcf6b2`)
- feat: app logo (`3d85385`)
- feat: custom favicon (`a569fc2`)
- feat: ansi codes (`5945615`)
- feat: view by deployment (`cda8245`)
- fix: connection (`ad7db75`)
- feat: add ui (`b91f550`)
- refactor: convert to mono repo (`88695f9`)
- feat: initial working version (`a5c5cea`)
