# Image of the MCP server: `openarch mcp --read-only` on stdin and stdout.
#
# GoReleaser builds it (dockers_v2 in .goreleaser.yaml). Its build context holds
# the release binaries as <os>/<arch>/openarch, so a plain `docker build .` in
# the repository does not work: run `goreleaser release --snapshot --clean`.
#
# The connection comes from OPENARCH_URL, OPENARCH_USER and OPENARCH_PASSWORD.
# Run the image with other flags, for example --listen, by passing them as the
# command: docker run ... ghcr.io/alfrescolabs/openarch mcp --read-only --listen 0.0.0.0:8765
FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/openarch /usr/local/bin/openarch

# The container has no saved contexts and no OS credential store. Docker
# Model Runner answers on model-runner.docker.internal inside containers.
ENV OPENARCH_CONFIG_DIR=/tmp/openarch \
    OPENARCH_CREDENTIAL_STORE=file \
    OPENARCH_MODEL_URL=http://model-runner.docker.internal/engines/v1

# Self-describing image for the Docker MCP Gateway. The gateway reads only the
# descriptive fields and the environment variable names from this label; the
# values come from the catalog entry in examples/mcp-gateway.
LABEL io.docker.server.metadata='{"name":"openarch","title":"Hyland OpenArch (CLI for OpenArch)","description":"Search and read the documents of Hyland OpenArch as one user, with that user'"'"'s permissions. Unofficial, from AlfrescoLabs.","readme":"https://github.com/AlfrescoLabs/openarch-cli/tree/main/examples/mcp-gateway","icon":"https://avatars.githubusercontent.com/u/16133585?s=200&v=4","env":[{"name":"OPENARCH_URL"},{"name":"OPENARCH_USER"},{"name":"OPENARCH_PASSWORD"}]}'

ENTRYPOINT ["/usr/local/bin/openarch"]
CMD ["mcp", "--read-only"]
