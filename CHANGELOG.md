# Change Log

All notable changes to this project will be documented in this file.
This project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

## [1.1.3] 2025-12-24

### Changed

- Update dependencies for Kubernetes 1.33 in [#10](https://github.com/cybozu-go/nyallocator/pull/10)
    - Update modules in go.mod
    - Use Golang 1.25 for building
    - Use Kubernetes 1.33 for testing
    - Update GitHub Actions

## [1.1.2] 2025-10-30

### Changed

- Fix condition of Reconciler triggers in [#8](https://github.com/cybozu-go/nyallocator/pull/8)

## [1.1.1] 2025-09-17

### Changed

- fix incorrect VAP and unstable test in [#6](https://github.com/cybozu-go/nyallocator/pull/6)

## [1.1.0] 2025-07-30

### Changed

- add metrics of spare nodes and status of reconciliation in [#4](https://github.com/cybozu-go/nyallocator/pull/4)

## [1.0.0] 2025-07-08

### Changed

- first release in [#1](https://github.com/cybozu-go/nyallocator/pull/1)

[Unreleased]: https://github.com/cybozu-go/nyallocator/compare/v1.1.3...HEAD
[1.1.3]: https://github.com/cybozu-go/nyallocator/compare/v1.1.2...v1.1.3
[1.1.2]: https://github.com/cybozu-go/nyallocator/compare/v1.1.1...v1.1.2
[1.1.1]: https://github.com/cybozu-go/nyallocator/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/cybozu-go/nyallocator/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/cybozu-go/nyallocator/compare/43fd6a4d6ae34f05fc74c0ba9165574c84f0638f...v1.0.0
