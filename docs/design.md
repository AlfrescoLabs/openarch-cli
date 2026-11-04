# Design decisions

## Content Lake integration

The CLI wraps the upstream repository
[content-lake-app-deployment](https://github.com/AlfrescoLabs/content-lake-app-deployment).
It does not embed its own copy of the Content Lake templates.

How it works:

- `openarch init --with content-lake` downloads a pinned ref of the upstream repository into the workspace.
- The CLI drives the upstream `compose.yaml` with Compose profiles and a generated `.env.local`.
- The pinned ref is stored in `openarch.yaml`. A newer ref is a CLI update, or an explicit `--content-lake-ref`.

Reasons:

- Content Lake is a proof of concept and changes often. Copied templates would drift from upstream.
- The upstream stack uses many Compose files, profiles and remote BuildKit contexts. Wrapping it keeps that logic in one place.
- The upstream Makefile has no visible license, so the CLI does not copy it. The CLI runs `docker compose` directly instead.

Costs:

- The CLI depends on the layout of the upstream repository at the pinned ref. Each ref bump needs a test run.
- `init --with content-lake` needs network access to download the ref.

## Alfresco for Content Lake

The CLI supports two Alfresco setups, in this order:

1. The Alfresco bundled with the Content Lake stack. This comes first.
2. An existing Alfresco installation. The CLI then installs the `content-lake-repo-model` module into it and connects the stack to it. This comes after the bundled setup works.
