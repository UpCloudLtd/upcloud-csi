# Releasing

1. Merge all your changes to the stable branch
3. Update CHANGELOG.md
   1. Add new heading with the correct version e.g. `## [1.0.1]`
   2. Update links at the bottom of the page
   3. Leave `## Unreleased` section at the top empty
5. Test GoReleaser config with `goreleaser check`
6. Tag a commit with the version you want to release e.g. `v1.2.0`
7. Push the tag & commit to GitHub
   - GitHub action automatically
      - sets the version based on the tag
      - creates a draft release to GitHub
8. Verify that [release notes](https://github.com/UpCloudLtd/upcloud-csi/releases) are in line with `CHANGELOG.md`
9. Publish the drafted release
