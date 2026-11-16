# Search as one user with a local agent and the Agent Skills

An AI agent with a shell answers the same question as two users of the permission demo, alice and
bob. The agent gets no OpenArch facts in its instruction: it learns how to search as one user from
the `openarch-search` skill that `openarch skill install` writes to `.agents/skills/` and
`.claude/skills/`. The model runs on your machine with Docker Model Runner: no cloud model and no
API key.

`run.sh` makes a new demo workspace in a temp folder, loads the demo documents with vectors, checks
them with `openarch demo check --semantic`, and asks the agent "How much is the pay for each job
level?" with the context of alice, then of bob. Then it checks the answers itself, and always
deletes the stack at the end.

## Requirements

- Docker Desktop with Docker Model Runner enabled, with host TCP access (`docker desktop enable model-runner --tcp 12434`)
- [Docker Agent](https://github.com/docker/docker-agent) (`docker agent version`)
- `openarch` on your `PATH`, or `OPENARCH=/path/to/openarch`
- Two local models: the embedding model of the demo, and a model that passes string arguments in tool
  calls. The default is GLM 4.7 Flash (about 16 GB):

```
docker model pull ai/mxbai-embed-large
docker model pull glm-4.7-flash:31B
```

The model runs next to the Hyland OpenArch stack, so plan for about 25 GB of free memory.

## Run

```
./run.sh
MODEL=<another model> PORT=<free port> ./run.sh
```

The script prints the result of `demo check`, the answer of each agent, and then `PASS` or `FAIL`
from its own check. The exit code is 0 only when alice's answer uses the salary bands and bob's
answer has neither their title nor their reference code: alice may read them, bob may not.

## How it works

- `agent.yaml` gives the agent `skills: true` and one toolset: `shell`. The agent loads the
  `openarch-search` skill with `read_skill` when the task names an openarch context.
- The skill tells the agent to use only the context of the user who asks, never another context,
  to cite the title and id of each document, and to say plainly when the user can read nothing on
  the topic.
- `run.sh` uses `--safety restricted`, and `agent.yaml` allows only `openarch ...` commands, with no
  pipes, redirections or second commands.
- `run.sh` sets `OPENARCH_CONFIG_DIR` to a temp folder, so the agent never touches your saved
  contexts or the OS credential store.
- The demo documents have metadata only (title and description), so the agent finds the right
  document but cannot quote figures from it.
