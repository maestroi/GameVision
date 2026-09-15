# Contributing

GameVision uses GitHub Actions to validate changes before they land on `main`.

## Pull request flow

1. Open a non-draft pull request targeting `main`.
2. CI checks formatting, module integrity, `go vet`, race-enabled tests, and a build.
3. Pull requests authored by the repository owner are automatically squash-merged after CI succeeds.

Add `[no-automerge]` to the pull request title when you want a successful PR to remain open for manual review.

Pull requests from other authors are never automatically merged by the workflow.
