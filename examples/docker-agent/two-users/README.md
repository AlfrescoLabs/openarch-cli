# One agent, two users

One AI agent answers the same question for two users of the permission demo, alice and bob, and
gives each of them a different answer. The agent searches Hyland OpenArch through `openarch mcp`,
which logs in as the user named in `OPENARCH_CONTEXT`. Hyland OpenArch returns only the documents
that this user may read, so the permissions come from the server, not from the prompt. The model
runs on your machine with Docker Model Runner: no cloud model and no API key.

The agent is also on Docker Hub as `angelborroy/openarch-agent`, so you can run it without this
folder.

## Requirements

- Docker Desktop with Docker Model Runner enabled, with host TCP access (`docker desktop enable model-runner --tcp 12434`)
- [Docker Agent](https://github.com/docker/docker-agent) (`docker agent version`)
- `openarch` on your `PATH`
- Two local models: the embedding model of the demo, and a model that passes string arguments in tool
  calls. The default is GLM 4.7 Flash (about 16 GB):

```
docker model pull ai/mxbai-embed-large
docker model pull glm-4.7-flash:31B
```

The model runs next to the Hyland OpenArch stack, so plan for about 25 GB of free memory.

## Try it

Start the permission demo with vectors. Run `demo users` before `up`: Hyland OpenArch reads its
users only at startup.

```
openarch init --dir oa-agent --context-name agent --port 18089 --non-interactive
openarch demo users --dir oa-agent
openarch up --dir oa-agent
openarch demo load --dir oa-agent --embeddings
```

`demo users` saves a context for each demo user: `agent-alice`, `agent-bob` and `agent-carol`. Ask
the same question as alice, then as bob:

```
OPENARCH_CONTEXT=agent-alice docker agent run angelborroy/openarch-agent --exec \
  "How much is the pay for each job level?"
OPENARCH_CONTEXT=agent-bob docker agent run angelborroy/openarch-agent --exec \
  "How much is the pay for each job level?"
```

alice may read the HR folder, so her answer names the document "Salary bands 2026". bob may not
read it, so his answer does not. Use `./agent.yaml` in place of `angelborroy/openarch-agent` to run
the copy in this folder. Delete the demo with `openarch clean --dir oa-agent --yes`.

## Test

```
./run.sh
AGENT=angelborroy/openarch-agent ./run.sh
MODEL=<another model> PORT=<free port> ./run.sh
```

`run.sh` makes a new demo workspace in a temp folder, checks it with `openarch demo check --semantic`,
and asks the agent the question as alice, then as bob. It prints both answers, then `PASS` or `FAIL`
from its own check, and always deletes the stack at the end. The exit code is 0 only when alice's
answer uses the salary bands and bob's answer has neither their title nor their reference code.

## How it works

- `agent.yaml` has one toolset: `openarch mcp` over stdio, with only four read-only tools: `query`,
  `documents_get`, `documents_list` and `config_whoami`.
- The MCP server logs in with the context in `OPENARCH_CONTEXT`. Its tools take no context, URL,
  user or password, so the agent cannot search as anyone else.
- The demo documents have metadata only (title and description), so the agent finds the right
  document but cannot quote figures from it.
