# End-to-end test framework for Crossplane providers

 `xp-testing` is a library enabling end-to-end tests for Crossplane providers, based on 
 [kubernetes-sigs/e2e-framework](https://github.com/kubernetes-sigs/e2e-framework/).

The testing framework helps to set up test suites, by handling the deployments of crossplane and providers & ensures 
providers are loaded into the cluster & helpers to speedup test development.

* `pkg/xpconditions` supports with assertions
* `pkg/resources` helps with handling of importing and deleting of resources while testing & an opinionated way to 
  create Test Features
* `pkg/setup` provides a default cluster setup, ready to take just the most necessary information and boostrap the 
  test suite
* `pkg/xpenvfuncs` provide basic functions to compose a test environment

 
## Getting Started and Documentation

For getting started guides, installation, deployment, and administration, check latest
Crossplane [document](https://crossplane.io/docs/latest).

A reference implementation of `xp-testing` is available in [provider-argocd](https://github.com/crossplane-contrib/provider-argocd/pull/89/files).

## Contributing

xp-testing is a community driven project and we welcome contributions. See the
Crossplane
[Contributing](https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md)
guidelines to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, please
open an [issue](https://github.com/crossplane-contrib/xp-testing/issues).

## Contact

Please use the following to reach members of the community:

* Slack: Join our [slack channel](https://slack.crossplane.io)
* Forums:
  [crossplane-dev](https://groups.google.com/forum/#!forum/crossplane-dev)
* Twitter/X: [@crossplane_io](https://twitter.com/crossplane_io)
* Email: [info@crossplane.io](mailto:info@crossplane.io)

## Governance and Owners

`xp-testing` is run according to the same
[Governance](https://github.com/crossplane/crossplane/blob/master/GOVERNANCE.md)
and [Ownership](https://github.com/crossplane/crossplane/blob/master/OWNERS.md)
structure as the core Crossplane project.

## Code of Conduct

`xp-testing` adheres to the same [Code of
Conduct](https://github.com/crossplane/crossplane/blob/master/CODE_OF_CONDUCT.md)
as the core Crossplane project.

## Licensing

xp-testing is under the Apache 2.0 license.

## Credits

Initially developed by [v0lkc](https://github.com/v0lkc), [mirzakopic](https://github.com/mirzakopic) and their team 
at [SAP](https://github.com/SAP/).
