Release procedure
=================

This document describes how to release a new version of nyallocator.

Versioning
----------

Follow [semantic versioning 2.0.0][semver] to choose the new version number.

Prepare change log entries
--------------------------

Add notable changes since the last release to [CHANGELOG.md](CHANGELOG.md).
It should look like:

```markdown
(snip)
## [Unreleased]

### Added
- Implement ... (#35)

### Changed
- Fix a bug in ... (#33)

### Removed
- Deprecated `-option` is removed ... (#39)

(snip)
```

Bump version
------------

1. Determine a new version number. Then set `VERSION` variable.

    ```console
    # Set VERSION and confirm it. It should not have "v" prefix.
    $ VERSION=x.y.z
    $ echo $VERSION
    ```

2. Make a branch to release

    ```console
    $ git checkout main
    $ git pull
    $ git checkout -b "bump-$VERSION"
    ```

3. Edit `CHANGELOG.md` for the new version.
4. Commit the change and push it.

    ```console
    $ git commit -a -m "Bump version to $VERSION"
    $ git push -u origin HEAD
    $ gh pr create -f
    ```

5. Merge this branch.
6. Add a git tag to the main HEAD, then push it.

    ```console
    # Set VERSION again.
    $ VERSION=x.y.z
    $ echo $VERSION

    $ git checkout main
    $ git pull
    $ git tag -a -m "Release v$VERSION" "v$VERSION"

    # Make sure the release tag exists.
    $ git tag -ln | grep $VERSION

    $ git push origin "v$VERSION"
    ```

GitHub actions will build and push artifacts such as container images and
create a new GitHub release.

[semver]: https://semver.org/spec/v2.0.0.html
