# Changelog

All notable changes to this project will be documented in this file.
See updating [Changelog example here](https://keepachangelog.com/en/1.0.0/)

## [Unreleased]

## [1.2.0]

### Added
- controller: support for standard storage tier

### Changed
- update upcloud-go-api to v8.6.1
- update Go to 1.22
- update Docker image to Alpine 3.20

## [1.1.0]

### Added

- controller: support for data at rest encryption (using encrypted snapshots as volume source is not supported yet)

## [1.0.1]

### Fixed
- controller: regard volume as unpublished from the node, if node is not found

## [1.0.0]

First stable release

[Unreleased]: https://github.com/UpCloudLtd/upcloud-csi/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/UpCloudLtd/upcloud-csi/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/UpCloudLtd/upcloud-csi/compare/v1.0.1...v1.1.0
[1.0.1]: https://github.com/UpCloudLtd/upcloud-csi/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/UpCloudLtd/upcloud-csi/releases/tag/1.0.0
