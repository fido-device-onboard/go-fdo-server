Releasing a new version
=======================

We will use the `v0.0.1` release as an example of how to release a new version:

* Fork the repo and create a new branch for the new release:

    ```bash
    gh repo fork fido-device-onboard/go-fdo-server --clone --remote
    git pull upstream main
    git checkout -b prepare-v0.0.1
    ```

* Update the `build/package/rpm/go-fdo-server.spec` file and set the new version: `Version: 0.0.1`
   ```bash
   sed -i 's/^Version:.*/Version: 0.0.1/' build/package/rpm/go-fdo-server.spec
   ```
* Verify the build by running `make`
* Verify the RPM build by running `make srpm rpm`
* Commit all the changes and create a PR:

    ```bash
    # go.mod and go.sum are included as `go mod tidy` (part of `make build`) might update dependencies.
    git add build/package/rpm/go-fdo-server.spec go.mod go.sum
    git commit -s -m "chore: bump for 0.0.1 release" -m "Prepare for the 0.0.1 release."
    gh pr create
    ```

* Once all the tests pass and the PR is merged, tag and sign the release:

    ```bash
    git tag -a -s v0.0.1
    git push upstream v0.0.1
    ```

* Using the webui, open the [Releases](https://github.com/fido-device-onboard/go-fdo-server/releases)
page and click the "Draft a new release" button in the middle of the page. From
there you can choose the `v0.0.1` tag you created in the previous step.
  * Use the version as the "Release title" and keep the format i.e. "v0.0.1".
  * In the description add in any release notes. When satisfied, click the
  "Save draft" or "Publish release" button at the bottom of the page.
