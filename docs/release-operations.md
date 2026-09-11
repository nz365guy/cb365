# Release operations

CB365 releases build six Go platform targets on a GitHub-hosted runner. Keep the GoReleaser invocation bounded with `--parallelism 1`; the CI release-safety check fails if that guardrail is removed or an unbounded `release --clean` invocation is restored.

If a hosted release stops during compilation:

1. Treat the provider's terminal status and raw job log as authoritative. A runner shutdown without a compiler diagnostic is not proof of a source-code failure or out-of-memory event.
2. Check for queued or in-progress runs of the same Release workflow and tag. Cancel only a proven duplicate; do not cancel an expected pull-request CI run or a distinct release tag.
3. Do not retry repeatedly. Diagnose the observed failure, make a durable workflow or source fix, and validate it with one corrected tagged release.
4. Require the corrected run, release assets, checksums, signature, SBOMs, and zero-active-duplicate readback before declaring the incident resolved.
5. Build and test on GitHub-hosted runners or a dedicated build machine. Do not compile CB365 on a production service host.
